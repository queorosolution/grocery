package payment

import (
	"mime"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
	"gp/internal/shared/idem"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Start handles POST /orders/{id}/payment. The key is checked before anything is read.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	key, err := idem.Key(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in StartInput
	routex.User(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.Start(r.Context(), sc, id, key, in)
	})
}

// MarkPaid handles POST /admin/orders/{id}/mark-paid.
func (h *Handler) MarkPaid(w http.ResponseWriter, r *http.Request) {
	var in MarkPaidInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.MarkPaid(r.Context(), sc, id, in)
	})
}

// form reads the callback body and leaves it readable for the next step.
func form(r *http.Request) (url.Values, error) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/x-www-form-urlencoded" {
		return nil, apperr.Validation(map[string]string{"contentType": "invalid"})
	}
	body, err := httpx.PeekBody(r, httpx.MaxWebhookBody)
	if err != nil {
		return nil, err
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, apperr.Validation(map[string]string{"body": "invalid"})
	}
	return values, nil
}

// VerifyNotify is the signature check of POST /webhooks/payhere. The route guard
// calls it before the handler (SigByRoute); it puts the body back for the handler.
func (h *Handler) VerifyNotify(r *http.Request) error {
	v, err := form(r)
	if err != nil {
		return err
	}
	return h.svc.Verify(r.Context(), v)
}

// Notify handles POST /webhooks/payhere. It answers 200 for everything that is
// not our error, so PayHere does not retry a callback we have already judged.
func (h *Handler) Notify(w http.ResponseWriter, r *http.Request) {
	v, err := form(r)
	if err == nil {
		err = h.svc.HandleNotify(r.Context(), v)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
