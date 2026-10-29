package delivery

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Rules handles GET /admin/fee-rules.
func (h *Handler) Rules(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		rules, err := h.svc.Rules(r.Context(), sc)
		return map[string]any{"data": rules}, err
	})
}

// SetDefaultRule handles PUT /admin/fee-rules/default.
func (h *Handler) SetDefaultRule(w http.ResponseWriter, r *http.Request) {
	in := DefaultRule()
	routex.Admin(w, r, http.StatusOK, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.SetRule(r.Context(), sc, nil, in)
	})
}

// SetShopRule handles PUT /admin/fee-rules/shops/{id}.
func (h *Handler) SetShopRule(w http.ResponseWriter, r *http.Request) {
	in := DefaultRule()
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.SetRule(r.Context(), sc, &id, in)
	})
}

// Preview handles POST /admin/fee-rules/preview.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var in PreviewInput
	routex.Admin(w, r, http.StatusOK, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.Preview(r.Context(), sc, in)
	})
}

// AdminSlots handles GET /admin/shops/{id}/slots?from=&to=.
func (h *Handler) AdminSlots(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		q := r.URL.Query()
		slots, err := h.svc.AdminSlots(r.Context(), sc, id, q.Get("from"), q.Get("to"))
		return map[string]any{"data": slots}, err
	})
}

// CreateSlots handles POST /admin/shops/{id}/slots.
func (h *Handler) CreateSlots(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slots []SlotInput `json:"slots"`
	}
	routex.Admin(w, r, http.StatusCreated, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		slots, err := h.svc.CreateSlots(r.Context(), sc, id, in.Slots)
		return map[string]any{"data": slots}, err
	})
}

// UpdateSlot handles PATCH /admin/slots/{id}.
func (h *Handler) UpdateSlot(w http.ResponseWriter, r *http.Request) {
	var in SlotPatch
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.UpdateSlot(r.Context(), sc, id, in)
	})
}

// CopyWeek handles POST /admin/shops/{id}/slots/copy-week.
func (h *Handler) CopyWeek(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WeekStart string `json:"weekStart"`
	}
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.CopyWeek(r.Context(), sc, id, in.WeekStart)
	})
}

// PublicSlots handles GET /shops/{slug}/slots?days=3. It is not cached: the
// remaining count changes with every order.
func (h *Handler) PublicSlots(w http.ResponseWriter, r *http.Request) {
	days := 3
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxPublicDays {
			httpx.Error(w, r, apperr.Validation(map[string]string{"days": "out_of_range"}))
			return
		}
		days = n
	}
	slots, err := h.svc.PublicSlots(r.Context(), r.PathValue("slug"), days)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": slots})
}
