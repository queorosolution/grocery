package user

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"gp/internal/auth"
	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
	"gp/internal/shared/routex"
)

// MeReader builds the "me" object the client expects after a change.
// *auth.Service is the real one.
type MeReader interface {
	Me(ctx context.Context, a authn.Actor) (auth.Me, error)
}

type Handler struct {
	svc *Service
	me  MeReader
}

func NewHandler(svc *Service, me MeReader) *Handler { return &Handler{svc: svc, me: me} }

// currentMe loads the "me" object for the caller.
func (h *Handler) currentMe(r *http.Request) (any, error) {
	actor, ok := authn.From(r.Context())
	if !ok {
		return nil, apperr.New(apperr.Unauthenticated)
	}
	return h.me.Me(r.Context(), actor)
}

// SetName handles PATCH /me.
func (h *Handler) SetName(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	routex.User(w, r, http.StatusOK, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		if err := h.svc.SetName(r.Context(), sc, in.Name); err != nil {
			return nil, err
		}
		return h.currentMe(r)
	})
}

// SendPhoneCode handles POST /me/phone/send.
func (h *Handler) SendPhoneCode(w http.ResponseWriter, r *http.Request) {
	var in PhoneCodeInput
	routex.User(w, r, http.StatusNoContent, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return nil, h.svc.SendPhoneCode(r.Context(), sc, in, httpx.ClientIP(r))
	})
}

// VerifyPhoneCode handles POST /me/phone/verify.
func (h *Handler) VerifyPhoneCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	routex.User(w, r, http.StatusOK, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		if err := h.svc.VerifyPhoneCode(r.Context(), sc, in.Code); err != nil {
			return nil, err
		}
		return h.currentMe(r)
	})
}

// Addresses handles GET /me/addresses.
func (h *Handler) Addresses(w http.ResponseWriter, r *http.Request) {
	routex.User[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		list, err := h.svc.Addresses(r.Context(), sc)
		return map[string]any{"data": list}, err
	})
}

// AddAddress handles POST /me/addresses.
func (h *Handler) AddAddress(w http.ResponseWriter, r *http.Request) {
	var in AddressInput
	routex.User(w, r, http.StatusCreated, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.AddAddress(r.Context(), sc, in)
	})
}

// UpdateAddress handles PATCH /me/addresses/{id}.
func (h *Handler) UpdateAddress(w http.ResponseWriter, r *http.Request) {
	var in AddressPatch
	routex.User(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.UpdateAddress(r.Context(), sc, id, in)
	})
}

// DeleteAddress handles DELETE /me/addresses/{id}.
func (h *Handler) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	routex.User[struct{}](w, r, http.StatusNoContent, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return nil, h.svc.DeleteAddress(r.Context(), sc, id)
	})
}
