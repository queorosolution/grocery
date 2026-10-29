package stats

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Shop handles GET /console/shops/{shopId}/dashboard?from=&to=.
func (h *Handler) Shop(w http.ResponseWriter, r *http.Request) {
	routex.Console[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		return h.svc.ShopDashboard(r.Context(), sc, q.Get("from"), q.Get("to"))
	})
}

// Admin handles GET /admin/dashboard?from=&to=.
func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		return h.svc.AdminDashboard(r.Context(), sc, q.Get("from"), q.Get("to"))
	})
}
