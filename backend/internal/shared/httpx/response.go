package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"gp/internal/shared/apperr"
	"gp/internal/shared/logx"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      apperr.Code       `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"requestId,omitempty"`
}

// JSON writes v as the response body. The body is built first, so a value that
// cannot be encoded gives a clean 500 instead of a half-written response.
func JSON(w http.ResponseWriter, status int, v any) {
	body, ok := encode(w, v)
	if !ok {
		return
	}
	writeBody(w, status, body)
}

func encode(w http.ResponseWriter, v any) ([]byte, bool) {
	body, err := json.Marshal(v)
	if err != nil {
		logx.From(context.Background()).Error("response encode failed", "err", err)
		writeError(w, apperr.New(apperr.Internal), w.Header().Get(requestIDHeader))
		return nil, false
	}
	return body, true
}

func writeBody(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// Error writes err in the API error shape (docs/06 section 1). Anything that is
// not an *apperr.Error becomes INTERNAL; its cause goes to the log with the
// request id and never to the client.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	e := apperr.From(err)
	if e.Code == apperr.Internal {
		logx.From(r.Context()).Error("internal error", "err", err)
	}
	writeError(w, e, RequestID(r.Context()))
}

func writeError(w http.ResponseWriter, e *apperr.Error, requestID string) {
	body, _ := json.Marshal(errorBody{Error: errorDetail{
		Code: e.Code, Message: e.Code.Message(), Fields: e.Fields, RequestID: requestID,
	}})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.Code.HTTPStatus())
	_, _ = w.Write(body)
}

// CachePublic marks a response as cacheable by Cloudflare and browsers. Only
// public catalog GETs may call it (D26); everything else keeps no-store.
func CachePublic(w http.ResponseWriter, maxAgeSeconds int) {
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAgeSeconds))
}
