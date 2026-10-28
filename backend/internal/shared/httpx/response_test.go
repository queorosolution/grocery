package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gp/internal/shared/apperr"
	"gp/internal/shared/logx"
)

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, http.StatusCreated, map[string]any{"ok": true})
	if w.Code != 201 || w.Header().Get("Content-Type") != "application/json; charset=utf-8" || strings.TrimSpace(w.Body.String()) != `{"ok":true}` {
		t.Fatalf("got %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

func TestJSONUnencodableValueGivesCleanError(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, http.StatusOK, map[string]any{"bad": make(chan int)})
	if w.Code != 500 || !strings.Contains(w.Body.String(), `"INTERNAL"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) errorDetail {
	t.Helper()
	var b errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("not the error shape: %v: %s", err, w.Body.String())
	}
	return b.Error
}

func TestErrorShape(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/x", nil).WithContext(context.WithValue(context.Background(), requestIDKey, "req_123"))
	Error(w, r, apperr.Validation(map[string]string{"slotId": "full"}))
	got := decodeBody(t, w)
	if w.Code != 400 || got.Code != apperr.ValidationError || got.Fields["slotId"] != "full" || got.RequestID != "req_123" || got.Message == "" {
		t.Fatalf("got %d %+v", w.Code, got)
	}
}

func TestErrorUsesContractStatusAndMessage(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, httptest.NewRequest(http.MethodGet, "/x", nil), apperr.New(apperr.SlotFull))
	got := decodeBody(t, w)
	if w.Code != 409 || got.Message != "That time slot is full. Please pick another." {
		t.Fatalf("got %d %+v", w.Code, got)
	}
}

// INFRA-07: a forced failure shows only the generic message, the cause goes to the log.
func TestInternalErrorsHideTheCauseAndLogIt(t *testing.T) {
	var logs bytes.Buffer
	ctx := logx.With(context.Background(), logx.New(slog.LevelDebug, &logs))
	r := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	Error(w, r, errors.New(`pq: relation "orders" does not exist`))

	got := decodeBody(t, w)
	if w.Code != 500 || got.Code != apperr.Internal || strings.Contains(w.Body.String(), "orders") {
		t.Fatalf("cause leaked to the client: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(logs.String(), "relation") {
		t.Fatalf("cause missing from the log: %s", logs.String())
	}
}

func TestCachePublic(t *testing.T) {
	w := httptest.NewRecorder()
	CachePublic(w, 60)
	if w.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatal(w.Header())
	}
}
