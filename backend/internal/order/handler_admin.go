package order

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
	"gp/internal/shared/routex"
)

// ReportProblem handles POST /orders/{id}/tickets.
func (h *Handler) ReportProblem(w http.ResponseWriter, r *http.Request) {
	var in TicketInput
	routex.User(w, r, http.StatusCreated, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.ReportProblem(r.Context(), sc, id, in)
	})
}

// Tickets handles GET /admin/tickets?status=.
func (h *Handler) Tickets(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		tickets, err := h.svc.Tickets(r.Context(), sc, r.URL.Query().Get("status"))
		return map[string]any{"data": tickets}, err
	})
}

// ResolveTicket handles POST /admin/tickets/{id}/resolve.
func (h *Handler) ResolveTicket(w http.ResponseWriter, r *http.Request) {
	var in ResolveInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.ResolveTicket(r.Context(), sc, id, in)
	})
}

// AdminList handles GET /admin/orders?shopId=&status=&q=&limit=&cursor=.
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		var shopID uuid.UUID
		if raw := q.Get("shopId"); raw != "" {
			var err error
			if shopID, err = uuid.Parse(raw); err != nil {
				return nil, apperr.Validation(map[string]string{"shopId": "invalid"})
			}
		}
		statuses, err := StatusFilter(q.Get("status"))
		if err != nil {
			return nil, err
		}
		limit, err := httpx.PageLimit(q.Get("limit"), defaultLimit)
		if err != nil {
			return nil, err
		}
		return h.svc.AdminList(r.Context(), sc, shopID, statuses, q.Get("q"), limit, q.Get("cursor"))
	})
}

// AdminTransition handles POST /admin/orders/{id}/transition.
func (h *Handler) AdminTransition(w http.ResponseWriter, r *http.Request) {
	var in AdminMoveInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.AdminTransition(r.Context(), sc, id, in)
	})
}
