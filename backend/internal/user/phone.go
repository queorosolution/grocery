package user

import (
	"context"
	"encoding/hex"
	"regexp"
	"strconv"
	"time"

	"gp/internal/notify"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/crypt"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
	"gp/internal/shared/pepper"
	"gp/internal/shared/ratelimit"
	"gp/internal/shared/validate"
)

// Phone verification limits (docs/10 section 4.6, AUTHN-09, BIZ-13).
const (
	codeDigits     = 6
	codeLifetime   = 10 * time.Minute
	maxAttempts    = 5
	sendsPerUser   = 3 // per hour
	sendsPerPhone  = 5 // per day
	sendUserWindow = time.Hour
	sendDayWindow  = 24 * time.Hour
)

var codeRe = regexp.MustCompile(`^[0-9]{4,6}$`)

// SendPhoneCode checks the Turnstile token, applies the SMS caps and sends a new
// code to the phone. Only Sri Lankan numbers are accepted. The code is stored as
// an HMAC with the OTP pepper and is never logged.
func (s *Service) SendPhoneCode(ctx context.Context, scope db.Scope, in PhoneCodeInput, clientIP string) error {
	phone, ok := validate.NormalizePhone(in.Phone)
	if !ok {
		return apperr.Validation(map[string]string{"phone": "invalid"})
	}
	if !validate.IsMobile(phone) {
		return apperr.Validation(map[string]string{"phone": "not_mobile"})
	}
	if err := s.turnstile.Verify(ctx, in.TurnstileToken, clientIP); err != nil {
		return err
	}
	phoneKey, err := pepper.Hash(ctx, s.secrets, pepper.OTP, "phone", phone)
	if err != nil {
		return err
	}
	if err := ratelimit.Hit(ctx, s.db, "otp:user:"+scope.UserID.String(), sendsPerUser, sendUserWindow); err != nil {
		return err
	}
	if err := ratelimit.Hit(ctx, s.db, "otp:phone:"+hex.EncodeToString(phoneKey), sendsPerPhone, sendDayWindow); err != nil {
		return err
	}

	code := crypt.RandomDigits(codeDigits)
	hash, err := s.codeHash(ctx, scope, phone, code)
	if err != nil {
		return err
	}
	now := clock.Now()
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		return replaceCode(ctx, tx, scope.UserID, phone, hash, now.Add(codeLifetime), now)
	})
	if err != nil {
		return err
	}
	// After the commit: a refused SMS must not leave the code half-saved, and a
	// saved code without an SMS is harmless (nobody knows it).
	return s.notify.Send(ctx, notify.Message{
		To: phone, Template: notify.OTP, UserID: &scope.UserID,
		Vars: map[string]string{"platform": s.platform, "code": code},
	})
}

func (s *Service) codeHash(ctx context.Context, scope db.Scope, phone, code string) ([]byte, error) {
	return pepper.Hash(ctx, s.secrets, pepper.OTP, "code", scope.UserID.String(), phone, code)
}

// VerifyPhoneCode checks the code against the newest one sent. Five wrong tries
// lock that code (OTP_LOCKED from the sixth try on). A wrong try is committed
// before the error is returned, or the counter would roll back with it.
func (s *Service) VerifyPhoneCode(ctx context.Context, scope db.Scope, code string) error {
	if !codeRe.MatchString(code) {
		return apperr.Validation(map[string]string{"code": "invalid"})
	}
	var denied error
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		now := clock.Now()
		row, found, err := activeCode(ctx, tx, scope.UserID)
		if err != nil {
			return err
		}
		switch {
		case !found || !row.Expires.After(now):
			denied = apperr.New(apperr.OtpInvalid)
			return nil
		case row.Attempts >= maxAttempts:
			denied = apperr.New(apperr.OtpLocked)
			return nil
		}
		want, err := s.codeHash(ctx, scope, row.Phone, code)
		if err != nil {
			return err
		}
		if !crypt.Equal(want, row.Hash) {
			logx.From(ctx).Info("user.otp_wrong", "userId", scope.UserID.String(), "attempt", strconv.Itoa(row.Attempts+1))
			denied = apperr.New(apperr.OtpInvalid)
			return addAttempt(ctx, tx, row.ID)
		}
		return confirmPhone(ctx, tx, scope.UserID, row.ID, row.Phone, now)
	})
	if err != nil {
		return err
	}
	return denied
}
