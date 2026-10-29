package audit

import (
	"context"
	"net/http"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authz"
	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
	"gp/internal/shared/validate"
)

type Handler struct{ db *db.DB }

func NewHandler(d *db.DB) *Handler { return &Handler{db: d} }

// maxEntity and maxEntityID follow the column limits.
const (
	maxEntity   = 40
	maxEntityID = 64
)

// List handles GET /admin/audit?entity=&entityId=&cursor=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := authz.ScopeFrom(r.Context())
	if !ok {
		httpx.Error(w, r, apperr.New(apperr.Forbidden))
		return
	}
	qs := r.URL.Query()
	q := Query{Entity: qs.Get("entity"), EntityID: qs.Get("entityId")}
	err := validate.New().
		Check("entity", len(q.Entity) <= maxEntity, "too_long").
		Check("entityId", len(q.EntityID) <= maxEntityID, "too_long").
		Err()
	if err == nil {
		q.Cursor, err = ParseCursor(qs.Get("cursor"))
	}
	var page Page
	if err == nil {
		err = h.db.Tx(r.Context(), scope, func(ctx context.Context, tx *db.Tx) (err error) {
			page, err = List(ctx, tx, q)
			return err
		})
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}
