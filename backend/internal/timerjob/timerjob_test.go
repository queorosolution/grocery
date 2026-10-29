package timerjob

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"gp/internal/events"
)

type fakeOrders struct {
	calls []timerEvent
	err   error
}

func (f *fakeOrders) Expire(_ context.Context, id uuid.UUID, kind events.TimerKind, rev int) (bool, error) {
	f.calls = append(f.calls, timerEvent{OrderID: id, Kind: kind, RevisionNo: rev})
	return true, f.err
}

const orderID = "6f1e1b52-6f6a-4d52-8a43-0c7c0e9a2b11"

// The scheduler payload is passed on as it is.
func TestHandle_CallsExpire(t *testing.T) {
	f := &fakeOrders{}
	raw := json.RawMessage(`{"orderId":"` + orderID + `","kind":"payment","revisionNo":2}`)
	if err := New(f).Handle(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0].OrderID.String() != orderID || f.calls[0].Kind != events.TimerPayment || f.calls[0].RevisionNo != 2 {
		t.Errorf("calls = %+v", f.calls)
	}
}

// A malformed event can never succeed: it is dropped without a retry and without touching an order.
func TestHandle_DropsABadEvent(t *testing.T) {
	for _, raw := range []string{`{}`, `not json`, `null`,
		`{"orderId":"00000000-0000-0000-0000-000000000000","kind":"review","revisionNo":0}`,
		`{"orderId":"` + orderID + `","kind":"lunch","revisionNo":0}`,
		`{"orderId":"` + orderID + `","kind":"review","revisionNo":-1}`,
		`{"orderId":"` + orderID + `","kind":"review","revisionNo":0,"extra":1}`} {
		f := &fakeOrders{}
		if err := New(f).Handle(context.Background(), json.RawMessage(raw)); err != nil || len(f.calls) != 0 {
			t.Errorf("%s: err %v, calls %v; want dropped", raw, err, f.calls)
		}
	}
}

// A real failure is returned, so Lambda retries; the retry is safe because Expire re-reads the order.
func TestHandle_ReturnsRealFailures(t *testing.T) {
	f := &fakeOrders{err: errors.New("database down")}
	raw := json.RawMessage(`{"orderId":"` + orderID + `","kind":"review","revisionNo":0}`)
	if err := New(f).Handle(context.Background(), raw); err == nil {
		t.Error("a database failure was swallowed")
	}
}
