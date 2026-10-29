package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"gp/internal/shared/clock"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

const testIssuer = "https://api.example.test"

func TestAccessTokenRoundTrip(t *testing.T) {
	uid, sid := uuid.New(), uuid.New()
	raw, exp, err := signAccess(testKey, testIssuer, uid, sid, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := clock.Now().Add(AccessTTL); exp.Sub(want) > time.Second {
		t.Fatalf("expiry = %v, want about %v", exp, want)
	}
	gotUID, gotSID, totp, err := parseAccess(testKey, testIssuer, raw)
	if err != nil {
		t.Fatal(err)
	}
	if gotUID != uid || gotSID != sid || !totp {
		t.Fatalf("claims = %v %v %v", gotUID, gotSID, totp)
	}
}

// AUTHN-02: only HS256 with our issuer, audience and an expiry is accepted.
func TestAccessTokenRejected(t *testing.T) {
	uid, sid := uuid.New().String(), uuid.New().String()
	now := time.Now()
	base := func() accessClaims {
		return accessClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: uid, Issuer: testIssuer, Audience: jwt.ClaimStrings{Audience},
				IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			},
			SessionID: sid,
		}
	}
	sign := func(m jwt.SigningMethod, c accessClaims, key any) string {
		s, err := jwt.NewWithClaims(m, c).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	wrongIss, wrongAud, expired, noExp := base(), base(), base(), base()
	wrongIss.Issuer = "https://evil.example.test"
	wrongAud.Audience = jwt.ClaimStrings{"someone-else"}
	expired.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute))
	noExp.ExpiresAt = nil

	none := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + uid + `","sid":"` + sid + `","iss":"` + testIssuer + `","aud":"gp-web","exp":` + "4102444800" + `}`)),
		"",
	}, ".")
	forgedKey := []byte(rsaKey.PublicKey.N.String()) // what a confusion attacker would use as an HMAC key

	tests := map[string]string{
		"alg none":         none,
		"HS512":            sign(jwt.SigningMethodHS512, base(), testKey),
		"RS256":            sign(jwt.SigningMethodRS256, base(), rsaKey),
		"HS256 forged key": sign(jwt.SigningMethodHS256, base(), forgedKey),
		"wrong issuer":     sign(jwt.SigningMethodHS256, wrongIss, testKey),
		"wrong audience":   sign(jwt.SigningMethodHS256, wrongAud, testKey),
		"expired":          sign(jwt.SigningMethodHS256, expired, testKey),
		"no expiry":        sign(jwt.SigningMethodHS256, noExp, testKey),
		"garbage":          "not.a.jwt",
		"empty":            "",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := parseAccess(testKey, testIssuer, raw); err == nil {
				t.Fatal("token was accepted")
			}
		})
	}
	// The control: the same claims signed correctly pass.
	if _, _, _, err := parseAccess(testKey, testIssuer, sign(jwt.SigningMethodHS256, base(), testKey)); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
}

func TestParseKey(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(testKey)
	if _, err := parseKey(good); err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]string{
		"short":      base64.StdEncoding.EncodeToString([]byte("too-short")),
		"not base64": "%%%",
		"empty":      "",
	} {
		if _, err := parseKey(v); err == nil {
			t.Errorf("%s key accepted", name)
		}
	}
}

func TestMaskPhone(t *testing.T) {
	if got := maskPhone("+94771234567"); got != "077•••4567" {
		t.Fatalf("maskPhone = %q", got)
	}
	if got := maskPhone("junk"); got != "•••" {
		t.Fatalf("maskPhone(junk) = %q", got)
	}
}
