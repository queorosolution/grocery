package auth

import (
	"context"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/clock"
	"gp/internal/shared/config"
	"gp/internal/shared/crypt"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/ids"
	"gp/internal/shared/logx"
	"gp/internal/shared/pepper"
	"gp/internal/shared/ratelimit"
	"gp/internal/shared/secrets"
	"gp/internal/team"
)

const maxNameRunes = 80

type Service struct {
	cfg      *config.Config
	db       *db.DB
	secrets  secrets.Getter
	verifier IDTokenVerifier
	team     *team.Service
	// Asked before an account is deleted, and told to clear details after (delete.go).
	blockers []Blocker
	wipers   []Wiper
}

func NewService(cfg *config.Config, d *db.DB, s secrets.Getter, v IDTokenVerifier, t *team.Service) *Service {
	return &Service{cfg: cfg, db: d, secrets: s, verifier: v, team: t}
}

func (s *Service) jwtKey(ctx context.Context) ([]byte, error) {
	raw, err := s.secrets.Get(ctx, "jwt/key")
	if err != nil {
		return nil, err
	}
	return parseKey(raw)
}

// ipKey hashes an address with the pepper, so neither the sessions table nor
// the rate counters ever hold a raw IP.
func (s *Service) ipKey(ctx context.Context, ip string) ([]byte, error) {
	return pepper.Hash(ctx, s.secrets, pepper.IP, ip)
}

// SignIn verifies the Google token and opens a session. The IP limit runs
// first so a flood of junk tokens never reaches Google or the database.
func (s *Service) SignIn(ctx context.Context, idToken, inviteToken string, ci ClientInfo) (Me, Tokens, error) {
	ipHash, err := s.ipKey(ctx, ci.IP)
	if err != nil {
		return Me{}, Tokens{}, err
	}
	if err := ratelimit.Hit(ctx, s.db, "auth:ip:"+hex.EncodeToString(ipHash), signInLimit, signInWindow); err != nil {
		return Me{}, Tokens{}, err
	}
	g, err := s.verifier.Verify(ctx, idToken)
	if err != nil {
		logx.From(ctx).Info("auth.google_rejected", "err", err)
		return Me{}, Tokens{}, apperr.New(apperr.Unauthenticated).Wrap(err)
	}
	subKey, err := s.ipKey(ctx, "sub:"+g.Sub)
	if err != nil {
		return Me{}, Tokens{}, err
	}
	if err := ratelimit.Hit(ctx, s.db, "auth:sub:"+hex.EncodeToString(subKey), signInLimit, signInWindow); err != nil {
		return Me{}, Tokens{}, err
	}

	email := strings.ToLower(strings.TrimSpace(g.Email))
	var me Me
	var tokens Tokens
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		u, err := upsertUser(ctx, tx, g, email, displayName(g.Name, email))
		if err != nil {
			return err
		}
		if u.Status != enum.UserStatusActive {
			return apperr.New(apperr.Forbidden)
		}
		if inviteToken != "" {
			if err := s.team.Accept(ctx, tx, u.ID, u.Email, inviteToken); err != nil {
				return err
			}
		}
		if slices.Contains(s.cfg.SuperAdminEmails, u.Email) {
			if err := ensureSuperAdmin(ctx, tx, u.ID); err != nil {
				return err
			}
		}
		_, tokens, err = s.open(ctx, tx, u.ID, uuid.New(), false, ci, ipHash)
		if err != nil {
			return err
		}
		me, err = s.loadMe(ctx, tx, u.ID, false)
		return err
	})
	if err != nil {
		return Me{}, Tokens{}, err
	}
	return me, tokens, nil
}

// Refresh swaps a refresh token for a new pair. A token that was already
// swapped means it was copied (AUTHN-03): the whole family is revoked and the
// alarm event is logged. Refusals commit their revocation, so the denial is
// returned after the transaction, not from inside it.
func (s *Service) Refresh(ctx context.Context, refresh string, ci ClientInfo) (Tokens, error) {
	if refresh == "" {
		return Tokens{}, apperr.New(apperr.Unauthenticated)
	}
	ipHash, err := s.ipKey(ctx, ci.IP)
	if err != nil {
		return Tokens{}, err
	}
	var tokens Tokens
	var denied error
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		sess, found, err := sessionByRefreshHash(ctx, tx, crypt.HashToken(refresh))
		if err != nil {
			return err
		}
		if !found {
			denied = apperr.New(apperr.Unauthenticated)
			return nil
		}
		switch {
		case sess.Rotated:
			logx.From(ctx).Warn("auth.refresh_reuse", "userId", sess.UserID.String(), "familyId", sess.FamilyID.String())
			denied = apperr.New(apperr.Unauthenticated)
			return revokeFamily(ctx, tx, sess.FamilyID)
		case sess.Revoked, !sess.ExpiresAt.After(clock.Now()):
			denied = apperr.New(apperr.Unauthenticated)
			return nil
		}
		u, ok, err := userByID(ctx, tx, sess.UserID)
		if err != nil {
			return err
		}
		if !ok || u.Status != enum.UserStatusActive {
			denied = apperr.New(apperr.Unauthenticated)
			return revokeFamily(ctx, tx, sess.FamilyID)
		}
		if err := markRotated(ctx, tx, sess.ID); err != nil {
			return err
		}
		_, tokens, err = s.open(ctx, tx, sess.UserID, sess.FamilyID, sess.TotpOK, ci, ipHash)
		return err
	})
	if err != nil {
		return Tokens{}, err
	}
	if denied != nil {
		return Tokens{}, denied
	}
	return tokens, nil
}

// Logout ends this sign-in, or every sign-in of the user.
func (s *Service) Logout(ctx context.Context, a authn.Actor, all bool) error {
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		if all {
			return revokeAllSessions(ctx, tx, a.UserID)
		}
		return revokeSessionFamily(ctx, tx, a.UserID, a.SessionID)
	})
}

// Me returns the signed-in user for /me and /bootstrap.
func (s *Service) Me(ctx context.Context, a authn.Actor) (Me, error) {
	var me Me
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		var err error
		me, err = s.loadMe(ctx, tx, a.UserID, a.TotpOK)
		return err
	})
	return me, err
}

// Authenticate reads the access cookie. A bad, expired or foreign token is just
// "not signed in": the route guard decides what that means for the route.
func (s *Service) Authenticate(r *http.Request) (authn.Actor, bool) {
	c, err := r.Cookie(AccessCookie)
	if err != nil || c.Value == "" {
		return authn.Actor{}, false
	}
	key, err := s.jwtKey(r.Context())
	if err != nil {
		logx.From(r.Context()).Error("auth.jwt_key_unavailable", "err", err)
		return authn.Actor{}, false
	}
	uid, sid, totp, err := parseAccess(key, s.cfg.APIBaseURL, c.Value)
	if err != nil {
		return authn.Actor{}, false
	}
	return authn.Actor{UserID: uid, SessionID: sid, TotpOK: totp}, true
}

// open stores a new session in the family and mints both tokens for it.
func (s *Service) open(ctx context.Context, tx *db.Tx, userID, familyID uuid.UUID, totpOK bool, ci ClientInfo, ipHash []byte) (uuid.UUID, Tokens, error) {
	key, err := s.jwtKey(ctx)
	if err != nil {
		return uuid.Nil, Tokens{}, err
	}
	refresh := ids.Token()
	refreshExp := clock.Now().Add(RefreshTTL)
	sessionID, err := insertSession(ctx, tx, newSession{
		UserID: userID, FamilyID: familyID, Hash: crypt.HashToken(refresh), TotpOK: totpOK,
		ExpiresAt: refreshExp, UserAgent: truncate(ci.UserAgent, 200), IPHash: ipHash,
	})
	if err != nil {
		return uuid.Nil, Tokens{}, err
	}
	access, accessExp, err := signAccess(key, s.cfg.APIBaseURL, userID, sessionID, totpOK)
	if err != nil {
		return uuid.Nil, Tokens{}, err
	}
	return sessionID, Tokens{Access: access, AccessExpires: accessExp, Refresh: refresh, RefreshExpires: refreshExp}, nil
}

// displayName uses the Google name, or the part of the e-mail before "@" when
// Google sent none, cut to the column limit.
func displayName(name, email string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	return truncate(name, maxNameRunes)
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}

// requireSuperAdmin refuses callers without the platform role. It reads the
// role from the database, never from the token.
func (s *Service) requireSuperAdmin(ctx context.Context, tx *db.Tx, a authn.Actor) error {
	ms, err := s.team.MembershipsTx(ctx, tx, a.UserID)
	if err != nil {
		return err
	}
	if !isSuperAdmin(ms) {
		return apperr.New(apperr.Forbidden)
	}
	return nil
}
