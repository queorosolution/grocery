package connector

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Create handles POST /admin/shops/{id}/connector.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in SecretInput
	routex.Admin(w, r, http.StatusCreated, true, &in, func(r *http.Request, sc db.Scope, shopID uuid.UUID) (any, error) {
		return h.svc.Create(r.Context(), sc, shopID, in)
	})
}

// ForShop handles GET /admin/shops/{id}/connector.
func (h *Handler) ForShop(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, shopID uuid.UUID) (any, error) {
		return h.svc.ForShop(r.Context(), sc, shopID)
	})
}

// ReplaceSecret handles PUT /admin/connectors/{id}/secret.
func (h *Handler) ReplaceSecret(w http.ResponseWriter, r *http.Request) {
	var in SecretInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.ReplaceSecret(r.Context(), sc, id, in)
	})
}

// Test handles POST /admin/connectors/{id}/test.
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Test(r.Context(), sc, id)
	})
}

// Tables handles GET /admin/connectors/{id}/tables.
func (h *Handler) Tables(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		tables, err := h.svc.Tables(r.Context(), sc, id)
		return map[string]any{"data": orEmpty(tables)}, err
	})
}

// Columns handles GET /admin/connectors/{id}/columns?table=.
func (h *Handler) Columns(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		cols, err := h.svc.Columns(r.Context(), sc, id, r.URL.Query().Get("table"))
		return map[string]any{"data": orEmpty(cols)}, err
	})
}

// SetMapping handles PUT /admin/connectors/{id}/mapping.
func (h *Handler) SetMapping(w http.ResponseWriter, r *http.Request) {
	var in MappingInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.SetMapping(r.Context(), sc, id, in)
	})
}

// Preview handles POST /admin/connectors/{id}/preview.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		rows, err := h.svc.Preview(r.Context(), sc, id)
		return map[string]any{"rows": rows}, err
	})
}

// orEmpty makes a nil list encode as [] and not null.
func orEmpty[T any](l []T) []T {
	if l == nil {
		return []T{}
	}
	return l
}
