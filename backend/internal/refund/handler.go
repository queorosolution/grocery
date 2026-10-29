package refund

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Request handles POST /console/shops/{shopId}/orders/{id}/refund-requests.
func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	var in RequestInput
	routex.Console(w, r, http.StatusCreated, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Request(r.Context(), sc, id, in)
	})
}

// List handles GET /admin/refunds?status=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		refunds, err := h.svc.List(r.Context(), sc, r.URL.Query().Get("status"))
		return map[string]any{"data": refunds}, err
	})
}

// Decide handles POST /admin/refunds/{id}/decide.
func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	var in DecideInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Decide(r.Context(), sc, id, in)
	})
}

// PaidOut handles POST /admin/refunds/{id}/paid-out.
func (h *Handler) PaidOut(w http.ResponseWriter, r *http.Request) {
	var in PaidOutInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.MarkPaidOut(r.Context(), sc, id, in)
	})
}
