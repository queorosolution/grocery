package coupon

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List handles GET /admin/coupons?status=&shopId=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		coupons, err := h.svc.List(r.Context(), sc, ListQuery{Status: q.Get("status"), ShopID: q.Get("shopId")})
		return map[string]any{"data": coupons}, err
	})
}

// Create handles POST /admin/coupons.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	routex.Admin(w, r, http.StatusCreated, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.Create(r.Context(), sc, in)
	})
}

// Update handles PATCH /admin/coupons/{id}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var in PatchInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Update(r.Context(), sc, id, in)
	})
}
