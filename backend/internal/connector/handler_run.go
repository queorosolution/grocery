package connector

import (
	"bytes"
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/httpx"
	"gp/internal/shared/routex"
)

// Activate handles POST /admin/connectors/{id}/activate.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Activate(r.Context(), sc, id)
	})
}

// Pause handles POST /admin/connectors/{id}/pause.
func (h *Handler) Pause(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Pause(r.Context(), sc, id)
	})
}

// Sync handles POST /admin/connectors/{id}/sync: 202, or CONNECTOR_BUSY.
func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusAccepted, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return map[string]bool{"queued": true}, h.svc.SyncNow(r.Context(), sc, id)
	})
}

// Runs handles GET /admin/connectors/{id}/runs.
func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		runs, err := h.svc.Runs(r.Context(), sc, id)
		return map[string]any{"data": orEmpty(runs)}, err
	})
}

// TriggerSecret handles POST /admin/connectors/{id}/trigger-secret. The secret
// is in this answer only.
func (h *Handler) TriggerSecret(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusCreated, true, nil, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.RotateTriggerSecret(r.Context(), sc, id)
	})
}

// Issues handles GET /admin/sync-issues?status=&shopId=.
func (h *Handler) Issues(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		var status *enum.SyncIssueStatus
		if v := r.URL.Query().Get("status"); v != "" {
			s := enum.SyncIssueStatus(v)
			if !s.Valid() {
				return nil, apperr.Validation(map[string]string{"status": "invalid"})
			}
			status = &s
		}
		var shopID *uuid.UUID
		if v := r.URL.Query().Get("shopId"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				return nil, apperr.Validation(map[string]string{"shopId": "invalid"})
			}
			shopID = &id
		}
		issues, err := h.svc.Issues(r.Context(), sc, status, shopID)
		return map[string]any{"data": orEmpty(issues)}, err
	})
}

// Resolve handles POST /admin/sync-issues/{id}/resolve.
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	var in ResolveInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Resolve(r.Context(), sc, id, in)
	})
}

// VerifyTrigger is the signature check of POST /connectors/{id}/trigger. The
// route guard calls it before the handler; it puts the body back so the handler
// can read it.
func (h *Handler) VerifyTrigger(r *http.Request) error {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return apperr.New(apperr.Unauthenticated)
	}
	body, err := httpx.PeekBody(r, httpx.MaxWebhookBody)
	if err != nil {
		return err
	}
	return h.svc.Authenticate(r.Context(), id, r.Header.Get("X-GP-Timestamp"), r.Header.Get("X-GP-Signature"), body)
}

// Trigger handles POST /connectors/{id}/trigger: 202 { "queued": true }.
func (h *Handler) Trigger(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		err = apperr.New(apperr.NotFound)
	}
	var in struct {
		ExternalIDs []string `json:"externalIds"`
	}
	if err == nil {
		// The body is optional; Decode refuses an empty one.
		var body []byte
		if body, err = httpx.PeekBody(r, httpx.MaxWebhookBody); err == nil && len(bytes.TrimSpace(body)) > 0 {
			err = httpx.Decode(r, &in)
		}
	}
	if err == nil {
		err = h.svc.Trigger(r.Context(), id, in.ExternalIDs)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}
