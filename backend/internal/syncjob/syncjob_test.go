package syncjob

import (
	"context"
	"encoding/json"
	"testing"
)

// A malformed event can never succeed, so it is dropped instead of failing the invoke (which Lambda would retry).
func TestHandle_DropsABadEvent(t *testing.T) {
	j := New(nil)
	for _, raw := range []string{`{}`, `not json`, `{"connectorId":"00000000-0000-0000-0000-000000000000","trigger":"SCHEDULE"}`,
		`{"connectorId":"6f1e1b52-6f6a-4d52-8a43-0c7c0e9a2b11","trigger":"NOPE"}`} {
		if err := j.Handle(context.Background(), json.RawMessage(raw)); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
}
