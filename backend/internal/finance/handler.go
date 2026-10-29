package finance

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Payouts }

func NewHandler(svc *Payouts) *Handler { return &Handler{svc: svc} }

// ShopPayouts handles GET /console/shops/{shopId}/payouts.
func (h *Handler) ShopPayouts(w http.ResponseWriter, r *http.Request) {
	routex.Console[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.ForShop(r.Context(), sc)
	})
}

// List handles GET /admin/payouts?shopId=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		payouts, err := h.svc.List(r.Context(), sc, r.URL.Query().Get("shopId"))
		return map[string]any{"data": payouts}, err
	})
}

// Create handles POST /admin/payouts.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	routex.Admin(w, r, http.StatusCreated, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.Create(r.Context(), sc, in)
	})
}

// Paid handles POST /admin/payouts/{id}/paid.
func (h *Handler) Paid(w http.ResponseWriter, r *http.Request) {
	var in PaidInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.MarkPaid(r.Context(), sc, id, in)
	})
}
