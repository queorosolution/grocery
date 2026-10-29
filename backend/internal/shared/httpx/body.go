package httpx

import (
	"bytes"
	"io"
	"net/http"

	"gp/internal/shared/apperr"
)

// PeekBody reads the body (at most limit bytes) and puts it back, so a signature
// check in the route guard and the handler after it can both read it.
func PeekBody(r *http.Request, limit int64) ([]byte, error) {
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, limit))
	if err != nil {
		return nil, apperr.New(apperr.PayloadTooLarge)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}
