package auth

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/clock"
	"gp/internal/shared/crypt"
	"gp/internal/shared/db"
	"gp/internal/shared/ratelimit"
	"gp/internal/shared/secrets"
	"gp/internal/shared/validate"
)

const (
	totpPeriod = 30 // seconds, the Google Authenticator default
	// totpSkew accepts one step either side of now for clock drift.
	totpSkew   = 1
	totpLimit  = 5
	totpWindow = 15 * time.Minute

	// totpKeyVersion is the encryption ring version for new secrets.
	totpKeyVersion byte = 1
)

var codeRe = regexp.MustCompile(`^[0-9]{6}$`)

// TotpSetup is shown once, as a QR code.
type TotpSetup struct {
	OtpauthURI   string `json:"otpauthUri"`
	SecretMasked string `json:"secretMasked"`
}

func (s *Service) ring(ctx context.Context) (*crypt.Ring, error) {
	return secrets.Ring(ctx, s.secrets, totpKeyVersion)
}

// totpAAD ties a sealed secret to its owner, so a row copied to another user
// cannot be opened.
func totpAAD(a authn.Actor) []byte { return []byte("admin_totp:" + a.UserID.String()) }

// TotpSetup creates a pending secret for a super admin who has none yet.
func (s *Service) TotpSetup(ctx context.Context, a authn.Actor) (TotpSetup, error) {
	ring, err := s.ring(ctx)
	if err != nil {
		return TotpSetup{}, err
	}
	var out TotpSetup
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		if err := s.requireSuperAdmin(ctx, tx, a); err != nil {
			return err
		}
		u, ok, err := userByID(ctx, tx, a.UserID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.New(apperr.Unauthenticated)
		}
		key, err := totp.Generate(totp.GenerateOpts{
			Issuer: s.cfg.PlatformName, AccountName: u.Email, Period: totpPeriod,
			Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
		})
		if err != nil {
			return fmt.Errorf("auth: generate totp secret: %w", err)
		}
		saved, err := saveTotpSecret(ctx, tx, a.UserID, ring.Seal([]byte(key.Secret()), totpAAD(a)))
		if err != nil {
			return err
		}
		if !saved {
			return apperr.New(apperr.Forbidden) // already set up
		}
		out = TotpSetup{OtpauthURI: key.URL(), SecretMasked: key.Secret()[:4] + "••••••••"}
		return nil
	})
	return out, err
}

// TotpVerify checks a code, turns TOTP on at the first success, and marks this
// session as verified. It returns a fresh access token carrying totp=true; the
// refresh token is unchanged because the session row remembers the check.
func (s *Service) TotpVerify(ctx context.Context, a authn.Actor, code string) (Me, Tokens, error) {
	if err := ratelimit.Hit(ctx, s.db, "totp:"+a.UserID.String(), totpLimit, totpWindow); err != nil {
		return Me{}, Tokens{}, err
	}
	if err := validate.New().Check("code", codeRe.MatchString(code), "invalid").Err(); err != nil {
		return Me{}, Tokens{}, err
	}
	ring, err := s.ring(ctx)
	if err != nil {
		return Me{}, Tokens{}, err
	}
	key, err := s.jwtKey(ctx)
	if err != nil {
		return Me{}, Tokens{}, err
	}
	var me Me
	var tokens Tokens
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		if err := s.requireSuperAdmin(ctx, tx, a); err != nil {
			return err
		}
		row, found, err := totpForUpdate(ctx, tx, a.UserID)
		if err != nil {
			return err
		}
		if !found {
			return apperr.New(apperr.Forbidden) // setup comes first
		}
		secret, err := ring.Open(row.SecretEnc, totpAAD(a))
		if err != nil {
			return fmt.Errorf("auth: open totp secret: %w", err)
		}
		step, ok := matchStep(string(secret), code, row.LastUsedStep)
		if !ok {
			return apperr.New(apperr.TotpInvalid)
		}
		if err := useTotpStep(ctx, tx, a.UserID, step); err != nil {
			return err
		}
		marked, err := markSessionTotp(ctx, tx, a.UserID, a.SessionID)
		if err != nil {
			return err
		}
		if !marked {
			return apperr.New(apperr.Unauthenticated)
		}
		access, exp, err := signAccess(key, s.cfg.APIBaseURL, a.UserID, a.SessionID, true)
		if err != nil {
			return err
		}
		tokens = Tokens{Access: access, AccessExpires: exp}
		me, err = s.loadMe(ctx, tx, a.UserID, true)
		return err
	})
	return me, tokens, err
}

// matchStep finds the time step the code belongs to, within the clock skew. A
// step at or before lastUsed is refused, so a code works once (AUTHN-06).
func matchStep(secret, code string, lastUsed int64) (int64, bool) {
	now := clock.Now().Unix() / totpPeriod
	for delta := int64(-totpSkew); delta <= totpSkew; delta++ {
		step := now + delta
		if step <= lastUsed {
			continue
		}
		want, err := hotp.GenerateCodeCustom(secret, uint64(step), hotp.ValidateOpts{ // #nosec G115 -- step is a positive time counter
			Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
		})
		if err == nil && crypt.Equal([]byte(want), []byte(code)) {
			return step, true
		}
	}
	return 0, false
}
