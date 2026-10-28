package pricing

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type markupsBody struct {
	Markups []Markup `json:"markups"`
}

// Markups handles GET /admin/shops/{id}/category-markups.
func (h *Handler) Markups(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		list, err := h.svc.Markups(r.Context(), sc, id)
		return map[string]any{"data": list}, err
	})
}

// SetMarkups handles PUT /admin/shops/{id}/category-markups.
func (h *Handler) SetMarkups(w http.ResponseWriter, r *http.Request) {
	var in markupsBody
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		list, err := h.svc.SetMarkups(r.Context(), sc, id, in.Markups)
		return map[string]any{"data": list}, err
	})
}
