package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"gp/internal/shared/clock"
)

// minKeyBytes is the shortest HMAC key we accept (docs/13 §2: 32+ bytes).
const minKeyBytes = 32

// accessClaims are the claims of our access token (docs/10 §4.1).
type accessClaims struct {
	jwt.RegisteredClaims
	SessionID string `json:"sid"`
	Totp      bool   `json:"totp"`
}

// parseKey decodes the base64 signing key from SSM and refuses a weak one.
func parseKey(b64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, errors.New("auth: jwt key is not base64")
	}
	if len(key) < minKeyBytes {
		return nil, errors.New("auth: jwt key is shorter than 32 bytes")
	}
	return key, nil
}

func signAccess(key []byte, issuer string, userID, sessionID uuid.UUID, totp bool) (string, time.Time, error) {
	now := clock.Now()
	exp := now.Add(AccessTTL)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		SessionID: sessionID.String(),
		Totp:      totp,
	})
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign access token: %w", err)
	}
	return signed, exp, nil
}

// parseAccess accepts only HS256 tokens with our issuer and audience and an
// expiry. Pinning the method closes the "alg: none" and RS-to-HS confusion
// attacks (AUTHN-02).
func parseAccess(key []byte, issuer, raw string) (userID, sessionID uuid.UUID, totp bool, err error) {
	var c accessClaims
	_, err = jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(clock.Now),
	).ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return key, nil })
	if err != nil {
		return uuid.Nil, uuid.Nil, false, err
	}
	if userID, err = uuid.Parse(c.Subject); err != nil {
		return uuid.Nil, uuid.Nil, false, err
	}
	if sessionID, err = uuid.Parse(c.SessionID); err != nil {
		return uuid.Nil, uuid.Nil, false, err
	}
	return userID, sessionID, c.Totp, nil
}
