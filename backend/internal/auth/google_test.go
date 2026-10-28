package auth_test

import (
	"context"
	"testing"
	"time"

	"gp/internal/auth"
	"gp/test/fakes"
)

const clientID = "test-client-id.apps.example.test"

// AUTHN-01: a Google token is accepted only for our client id, from the right
// issuer, unexpired, correctly signed and with a verified e-mail.
func TestGoogleVerifier(t *testing.T) {
	oidc := fakes.NewOIDC(t)
	v := auth.NewOIDCVerifier(oidc.Issuer, clientID)
	no := false

	t.Run("valid", func(t *testing.T) {
		raw := oidc.Token(t, clientID, fakes.Claims{Sub: "s1", Email: "A@Example.test", Name: "Ama"})
		got, err := v.Verify(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if got.Sub != "s1" || got.Email != "A@Example.test" || got.Name != "Ama" {
			t.Fatalf("identity = %+v", got)
		}
	})

	rejected := map[string]fakes.Claims{
		"token for another app": {Audience: "other-app.apps.example.test"},
		"expired":               {Expires: time.Now().Add(-time.Hour)},
		"unverified e-mail":     {EmailVerified: &no},
		"wrong issuer":          {Issuer: "https://evil.example.test"},
		"unknown signing key":   {SignedByOtherKey: true},
	}
	for name, c := range rejected {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Verify(context.Background(), oidc.Token(t, clientID, c)); err == nil {
				t.Fatal("token was accepted")
			}
		})
	}
	t.Run("garbage", func(t *testing.T) {
		if _, err := v.Verify(context.Background(), "not-a-token"); err == nil {
			t.Fatal("garbage was accepted")
		}
	})
}
