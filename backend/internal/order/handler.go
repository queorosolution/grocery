package order

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
	"gp/internal/shared/idem"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Quote handles POST /bills/quote.
func (h *Handler) Quote(w http.ResponseWriter, r *http.Request) {
	var in QuoteInput
	routex.User(w, r, http.StatusOK, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.Quote(r.Context(), sc, in)
	})
}

// Submit handles POST /bills. The key is checked before anything is read or priced.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	key, err := idem.Key(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in SubmitInput
	routex.User(w, r, http.StatusCreated, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.Submit(r.Context(), sc, key, in)
	})
}

// List handles GET /orders.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	routex.User[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		limit, err := httpx.PageLimit(r.URL.Query().Get("limit"), defaultLimit)
		if err != nil {
			return nil, err
		}
		return h.svc.List(r.Context(), sc, limit, r.URL.Query().Get("cursor"))
	})
}

// Get handles GET /orders/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	routex.User[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Get(r.Context(), sc, id)
	})
}

// AcceptRevision handles POST /orders/{id}/accept-revision.
func (h *Handler) AcceptRevision(w http.ResponseWriter, r *http.Request) {
	var in AcceptInput
	routex.User(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.AcceptRevision(r.Context(), sc, id, in)
	})
}

// Cancel handles POST /orders/{id}/cancel.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	var in CancelInput
	routex.User(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Cancel(r.Context(), sc, id, in)
	})
}

// ConsoleList handles GET /console/shops/{shopId}/orders. The answer has an ETag.
func (h *Handler) ConsoleList(w http.ResponseWriter, r *http.Request) {
	routex.Console[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		statuses, err := StatusFilter(q.Get("status"))
		if err != nil {
			return nil, err
		}
		limit, err := httpx.PageLimit(q.Get("limit"), defaultLimit)
		if err != nil {
			return nil, err
		}
		page, err := h.svc.ConsoleList(r.Context(), sc, statuses, limit, q.Get("cursor"))
		return httpx.Revalidated{V: page}, err
	})
}

// ConsoleGet handles GET /console/shops/{shopId}/orders/{id}.
func (h *Handler) ConsoleGet(w http.ResponseWriter, r *http.Request) {
	routex.Console[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Get(r.Context(), sc, id)
	})
}

// Review handles POST /console/shops/{shopId}/orders/{id}/review.
func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	var in ReviewInput
	routex.Console(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Review(r.Context(), sc, id, in)
	})
}
