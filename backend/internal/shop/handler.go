package shop

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/adminapi"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// publicCacheSeconds is how long Cloudflare may keep the public shop list. A
// new shop also triggers a site rebuild, so a short time is enough.
const publicCacheSeconds = 60

// PublicList handles GET /shops?fields=slug.
func (h *Handler) PublicList(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("fields") != "slug" {
		httpx.Error(w, r, apperr.Validation(map[string]string{"fields": "invalid"}))
		return
	}
	shops, err := h.svc.PublicList(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.CachePublic(w, publicCacheSeconds)
	httpx.JSON(w, http.StatusOK, map[string]any{"data": shops})
}

// List handles GET /admin/shops.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	adminapi.Handle[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		shops, err := h.svc.List(r.Context(), sc)
		return map[string]any{"data": shops}, err
	})
}

// Create handles POST /admin/shops.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	adminapi.Handle(w, r, http.StatusCreated, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.Create(r.Context(), sc, in)
	})
}

// Get handles GET /admin/shops/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	adminapi.Handle[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Get(r.Context(), sc, id)
	})
}

// Update handles PATCH /admin/shops/{id}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	adminapi.Handle(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Update(r.Context(), sc, id, in)
	})
}

// PutSettings handles PUT /admin/shops/{id}/settings.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	adminapi.Handle(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.UpdateSettings(r.Context(), sc, id, in)
	})
}

type hoursBody struct {
	Days []Day `json:"days"`
}

// PutHours handles PUT /admin/shops/{id}/hours.
func (h *Handler) PutHours(w http.ResponseWriter, r *http.Request) {
	var in hoursBody
	adminapi.Handle(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.SetHours(r.Context(), sc, id, in.Days)
	})
}

type acceptingBody struct {
	On bool `json:"on"`
}

// Accepting handles POST /admin/shops/{id}/accepting.
func (h *Handler) Accepting(w http.ResponseWriter, r *http.Request) {
	var in acceptingBody
	adminapi.Handle(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.SetAccepting(r.Context(), sc, id, in.On)
	})
}

// Readiness handles GET /admin/shops/{id}/readiness.
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	adminapi.Handle[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Readiness(r.Context(), sc, id)
	})
}

// Accounts handles GET /admin/payment-accounts.
func (h *Handler) Accounts(w http.ResponseWriter, r *http.Request) {
	adminapi.Handle[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		list, err := h.svc.Accounts(r.Context(), sc)
		return map[string]any{"data": list}, err
	})
}

// CreateAccount handles POST /admin/payment-accounts.
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var in CreateAccountInput
	adminapi.Handle(w, r, http.StatusCreated, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.CreateAccount(r.Context(), sc, in)
	})
}

// UpdateAccount handles PATCH /admin/payment-accounts/{id}.
func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	var in UpdateAccountInput
	adminapi.Handle(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.UpdateAccount(r.Context(), sc, id, in)
	})
}
