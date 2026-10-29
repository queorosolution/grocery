package order

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

// Status handles POST /console/shops/{shopId}/orders/{id}/status (T13, T14, T15, T18).
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	var in StatusInput
	routex.Console(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Fulfil(r.Context(), sc, id, in)
	})
}

// ConsoleDriver handles POST /console/shops/{shopId}/orders/{id}/driver.
func (h *Handler) ConsoleDriver(w http.ResponseWriter, r *http.Request) {
	var in AssignInput
	routex.Console(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.AssignDriver(r.Context(), sc, id, in)
	})
}

// ConsoleDrivers handles GET /console/shops/{shopId}/drivers.
func (h *Handler) ConsoleDrivers(w http.ResponseWriter, r *http.Request) {
	routex.Console[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		drivers, err := h.svc.Drivers(r.Context(), sc)
		return map[string]any{"data": drivers}, err
	})
}

// AdminDriver handles POST /admin/orders/{id}/driver: the admin may pick a platform driver.
func (h *Handler) AdminDriver(w http.ResponseWriter, r *http.Request) {
	var in AssignInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.AssignDriver(r.Context(), sc, id, in)
	})
}

// UnlockCode handles POST /admin/orders/{id}/unlock-code.
func (h *Handler) UnlockCode(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.UnlockCode(r.Context(), sc, id)
	})
}

// Deliveries handles GET /driver/deliveries?day=. The driver is the signed-in user.
func (h *Handler) Deliveries(w http.ResponseWriter, r *http.Request) {
	routex.User[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		groups, err := h.svc.Deliveries(r.Context(), sc, r.URL.Query().Get("day"))
		return map[string]any{"data": groups}, err
	})
}

// Complete handles POST /driver/deliveries/{id}/complete (T16).
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	var in CompleteInput
	routex.User(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Complete(r.Context(), sc, id, in)
	})
}

// Fail handles POST /driver/deliveries/{id}/fail (T17).
func (h *Handler) Fail(w http.ResponseWriter, r *http.Request) {
	var in FailInput
	routex.User(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Fail(r.Context(), sc, id, in)
	})
}

// DriverStats handles GET /driver/stats?from=&to=.
func (h *Handler) DriverStats(w http.ResponseWriter, r *http.Request) {
	routex.User[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		return h.svc.Stats(r.Context(), sc, q.Get("from"), q.Get("to"))
	})
}
