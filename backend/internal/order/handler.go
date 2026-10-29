package order

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/db"
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
