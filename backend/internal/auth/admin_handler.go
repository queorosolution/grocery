package auth

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authz"
	"gp/internal/shared/enum"
	"gp/internal/shared/httpx"
)

// Users handles GET /admin/users?q=.
func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.SearchUsers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": users})
}

type statusBody struct {
	Status enum.UserStatus `json:"status"`
}

// SetUserStatus handles PATCH /admin/users/{id}.
func (h *Handler) SetUserStatus(w http.ResponseWriter, r *http.Request) {
	scope, ok := authz.ScopeFrom(r.Context())
	if !ok {
		httpx.Error(w, r, apperr.New(apperr.Forbidden))
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, apperr.New(apperr.NotFound))
		return
	}
	var in statusBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.SetUserStatus(r.Context(), scope, id, in.Status); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RestoreUser handles POST /admin/users/{id}/restore.
func (h *Handler) RestoreUser(w http.ResponseWriter, r *http.Request) {
	scope, ok := authz.ScopeFrom(r.Context())
	if !ok {
		httpx.Error(w, r, apperr.New(apperr.Forbidden))
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, apperr.New(apperr.NotFound))
		return
	}
	if err := h.svc.RestoreUser(r.Context(), scope, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
