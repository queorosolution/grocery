package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
)

// GoogleIssuer is the OpenID issuer for Google ID tokens.
const GoogleIssuer = "https://accounts.google.com"

// IDTokenVerifier checks a Google ID token. The service depends on this small
// interface so tests do not need the network.
type IDTokenVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (GoogleIdentity, error)
}

// OIDCVerifier verifies tokens against an issuer's published keys. go-oidc
// checks the signature, issuer, audience (our client id) and expiry; we add the
// verified-email rule (AUTHN-01).
type OIDCVerifier struct {
	issuer   string
	clientID string

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
}

// NewGoogleVerifier verifies tokens for our Google client id. The key set is
// fetched on first use, so start-up never waits on Google.
func NewGoogleVerifier(clientID string) *OIDCVerifier {
	return NewOIDCVerifier(GoogleIssuer, clientID)
}

// NewOIDCVerifier is the same for any issuer; tests point it at a local server.
func NewOIDCVerifier(issuer, clientID string) *OIDCVerifier {
	return &OIDCVerifier{issuer: issuer, clientID: clientID}
}

func (v *OIDCVerifier) get(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.verifier != nil {
		return v.verifier, nil
	}
	p, err := oidc.NewProvider(ctx, v.issuer)
	if err != nil {
		return nil, fmt.Errorf("auth: oidc discovery: %w", err)
	}
	v.verifier = p.Verifier(&oidc.Config{
		ClientID:             v.clientID,
		SupportedSigningAlgs: []string{oidc.RS256},
	})
	return v.verifier, nil
}

var errUnverifiedEmail = errors.New("auth: google email not verified")

func (v *OIDCVerifier) Verify(ctx context.Context, raw string) (GoogleIdentity, error) {
	ver, err := v.get(ctx)
	if err != nil {
		return GoogleIdentity{}, err
	}
	tok, err := ver.Verify(ctx, raw)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: verify id token: %w", err)
	}
	var c struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := tok.Claims(&c); err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: id token claims: %w", err)
	}
	if !c.EmailVerified || c.Email == "" {
		return GoogleIdentity{}, errUnverifiedEmail
	}
	return GoogleIdentity{Sub: tok.Subject, Email: c.Email, Name: c.Name}, nil
}
