package auth

import (
	"net/http"
	"time"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/config"
	"gp/internal/shared/httpx"
)

const (
	AccessCookie  = "gp_at"
	RefreshCookie = "gp_rt"

	// refreshPath keeps the long-lived cookie off every request except the
	// auth calls (AUTHN-04).
	refreshPath = httpBase + "/auth"
	httpBase    = "/v1"

	// A Google ID token is about 1 KB; this only stops absurd input early.
	maxIDTokenLen = 4096
)

type Handler struct {
	svc    *Service
	secure bool
}

func NewHandler(svc *Service, cfg *config.Config) *Handler {
	// Secure cookies are skipped only on plain-http local development.
	return &Handler{svc: svc, secure: !cfg.IsLocal()}
}

// Authenticate is the router's Authn hook.
func (h *Handler) Authenticate(r *http.Request) (authn.Actor, bool) { return h.svc.Authenticate(r) }

type googleBody struct {
	IDToken string `json:"idToken"`
}

// Google handles POST /auth/google.
func (h *Handler) Google(w http.ResponseWriter, r *http.Request) {
	var in googleBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if in.IDToken == "" || len(in.IDToken) > maxIDTokenLen {
		httpx.Error(w, r, apperr.Validation(map[string]string{"idToken": "invalid"}))
		return
	}
	me, tokens, err := h.svc.SignIn(r.Context(), in.IDToken, clientInfo(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	h.setCookies(w, tokens)
	httpx.JSON(w, http.StatusOK, me)
}

// Refresh handles POST /auth/refresh. A refused refresh also clears the
// cookies, so the browser stops sending a dead token.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var token string
	if c, err := r.Cookie(RefreshCookie); err == nil {
		token = c.Value
	}
	tokens, err := h.svc.Refresh(r.Context(), token, clientInfo(r))
	if err != nil {
		if apperr.Is(err, apperr.Unauthenticated) {
			h.clearCookies(w)
		}
		httpx.Error(w, r, err)
		return
	}
	h.setCookies(w, tokens)
	w.WriteHeader(http.StatusNoContent)
}

type logoutBody struct {
	All bool `json:"all"`
}

// Logout handles POST /auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.From(r.Context()) // the user guard already ran
	var in logoutBody
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	if err := h.svc.Logout(r.Context(), actor, in.All); err != nil {
		httpx.Error(w, r, err)
		return
	}
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /me.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.From(r.Context())
	me, err := h.svc.Me(r.Context(), actor)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, me)
}

// MeOrNil is for /bootstrap: a signed-out or refused caller simply has no "me".
func (h *Handler) MeOrNil(r *http.Request) (*Me, error) {
	actor, ok := authn.From(r.Context())
	if !ok {
		return nil, nil
	}
	me, err := h.svc.Me(r.Context(), actor)
	if apperr.Is(err, apperr.Unauthenticated) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &me, nil
}

func (h *Handler) setCookies(w http.ResponseWriter, t Tokens) {
	http.SetCookie(w, h.cookie(AccessCookie, t.Access, "/", t.AccessExpires))
	http.SetCookie(w, h.cookie(RefreshCookie, t.Refresh, refreshPath, t.RefreshExpires))
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	http.SetCookie(w, h.cookie(AccessCookie, "", "/", time.Unix(0, 0)))
	http.SetCookie(w, h.cookie(RefreshCookie, "", refreshPath, time.Unix(0, 0)))
}

// cookie sets no Domain, so the cookie is host-only on the API host.
func (h *Handler) cookie(name, value, path string, expires time.Time) *http.Cookie {
	c := &http.Cookie{ // #nosec G124 -- Secure is on everywhere except plain-http local development
		Name: name, Value: value, Path: path, Expires: expires,
		HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode,
	}
	if value == "" {
		c.MaxAge = -1
	}
	return c
}

func clientInfo(r *http.Request) ClientInfo {
	return ClientInfo{IP: httpx.ClientIP(r), UserAgent: r.UserAgent()}
}
