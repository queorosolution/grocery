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
