// Package fakes holds small stand-ins for outside services, used by tests only.
package fakes

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// OIDC is a local OpenID provider: discovery, a key set, and a way to mint ID
// tokens. It lets the Google verifier run its real signature, issuer, audience
// and expiry checks without the network.
type OIDC struct {
	Issuer string
	key    *rsa.PrivateKey
	other  *rsa.PrivateKey
	srv    *httptest.Server
}

func NewOIDC(t testing.TB) *OIDC {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	o := &OIDC{key: key, other: other}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                o.Issuer,
			"jwks_uri":                              o.Issuer + "/keys",
			"authorization_endpoint":                o.Issuer + "/auth",
			"token_endpoint":                        o.Issuer + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test-key",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	o.srv = httptest.NewServer(mux)
	o.Issuer = o.srv.URL
	t.Cleanup(o.srv.Close)
	return o
}

// Claims are the ID token fields a test may change. Zero values mean "normal".
type Claims struct {
	Sub           string
	Email         string
	Name          string
	EmailVerified *bool
	Audience      string
	Issuer        string
	Expires       time.Time
	// SignedByOtherKey signs with a key that is not in the key set.
	SignedByOtherKey bool
}

// Token mints an ID token for the client id with the given overrides.
func (o *OIDC) Token(t testing.TB, clientID string, c Claims) string {
	t.Helper()
	if c.Sub == "" {
		c.Sub = "google-sub-1"
	}
	if c.Email == "" {
		c.Email = "nimal@example.test"
	}
	if c.Audience == "" {
		c.Audience = clientID
	}
	if c.Issuer == "" {
		c.Issuer = o.Issuer
	}
	if c.Expires.IsZero() {
		c.Expires = time.Now().Add(time.Hour)
	}
	verified := true
	if c.EmailVerified != nil {
		verified = *c.EmailVerified
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": c.Issuer, "aud": c.Audience, "sub": c.Sub,
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": c.Expires.Unix(),
		"email": c.Email, "email_verified": verified, "name": c.Name,
	})
	tok.Header["kid"] = "test-key"
	signer := o.key
	if c.SignedByOtherKey {
		signer = o.other
	}
	raw, err := tok.SignedString(signer)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
