package team

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authz"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// scope is what the route guard decided; a missing one means the route was
// registered without a guard, which the build test forbids, so it fails closed.
func scope(r *http.Request) (db.Scope, error) {
	s, ok := authz.ScopeFrom(r.Context())
	if !ok {
		return db.Scope{}, apperr.New(apperr.Forbidden)
	}
	return s, nil
}

func pathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, apperr.New(apperr.NotFound)
	}
	return id, nil
}

type inviteBody struct {
	Email           string    `json:"email"`
	Role            enum.Role `json:"role"`
	CanEditProducts bool      `json:"canEditProducts"`
}

type adminInviteBody struct {
	Email  string     `json:"email"`
	Role   enum.Role  `json:"role"`
	ShopID *uuid.UUID `json:"shopId"`
}

type changeBody struct {
	Role            *enum.Role `json:"role"`
	CanEditProducts *bool      `json:"canEditProducts"`
}

// List handles GET /console/shops/{shopId}/team.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	sc, err := scope(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	t, err := h.svc.List(r.Context(), sc.ShopID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// Invite handles POST /console/shops/{shopId}/team/invites.
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	sc, err := scope(r)
	var in inviteBody
	if err == nil {
		err = httpx.Decode(r, &in)
	}
	var out Issued
	if err == nil {
		out, err = h.svc.Invite(r.Context(), sc, sc.ShopID, InviteInput(in))
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

// Change handles PATCH /console/shops/{shopId}/team/{membershipId}.
func (h *Handler) Change(w http.ResponseWriter, r *http.Request) {
	sc, err := scope(r)
	var id uuid.UUID
	if err == nil {
		id, err = pathID(r, "membershipId")
	}
	var in changeBody
	if err == nil {
		err = httpx.Decode(r, &in)
	}
	if err == nil {
		err = h.svc.Change(r.Context(), sc, sc.ShopID, id, ChangeInput(in))
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Remove handles DELETE /console/shops/{shopId}/team/{membershipId}.
func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	sc, err := scope(r)
	var id uuid.UUID
	if err == nil {
		id, err = pathID(r, "membershipId")
	}
	if err == nil {
		err = h.svc.Remove(r.Context(), sc.ShopID, id)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdminInvite handles POST /admin/invites.
func (h *Handler) AdminInvite(w http.ResponseWriter, r *http.Request) {
	sc, err := scope(r)
	var in adminInviteBody
	if err == nil {
		err = httpx.Decode(r, &in)
	}
	var out Issued
	if err == nil {
		out, err = h.svc.AdminInvite(r.Context(), sc, in.ShopID, InviteInput{Email: in.Email, Role: in.Role})
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

// AdminRevokeInvite handles DELETE /admin/invites/{id}.
func (h *Handler) AdminRevokeInvite(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		err = h.svc.RevokeInvite(r.Context(), id)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
