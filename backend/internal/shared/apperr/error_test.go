package apperr

import (
	"errors"
	"fmt"
	"testing"
)

func TestFromKeepsAppErrors(t *testing.T) {
	err := fmt.Errorf("while booking: %w", New(SlotFull))
	if got := From(err); got.Code != SlotFull {
		t.Fatalf("code = %s, want SLOT_FULL", got.Code)
	}
	if !Is(err, SlotFull) || Is(err, Conflict) {
		t.Fatal("Is gives the wrong answer")
	}
}

func TestFromTurnsUnknownErrorsIntoInternal(t *testing.T) {
	cause := errors.New("pq: connection reset")
	got := From(cause)
	if got.Code != Internal {
		t.Fatalf("code = %s, want INTERNAL", got.Code)
	}
	if !errors.Is(got, cause) {
		t.Error("cause must stay reachable for logs")
	}
	if got.Code.Message() == cause.Error() {
		t.Error("the client message must not be the cause")
	}
}

func TestValidationCarriesFields(t *testing.T) {
	e := Validation(map[string]string{"name": "required"})
	if e.Code != ValidationError || e.Fields["name"] != "required" || e.Code.HTTPStatus() != 400 {
		t.Fatalf("unexpected %+v", e)
	}
}

func TestWrapDoesNotChangeTheOriginal(t *testing.T) {
	base := New(NotFound)
	_ = base.Wrap(errors.New("x"))
	if base.Unwrap() != nil {
		t.Fatal("Wrap changed the shared error")
	}
}

func TestNotFoundUnless(t *testing.T) {
	boom := errors.New("boom")
	if err := NotFoundUnless(boom, false); !errors.Is(err, boom) {
		t.Errorf("database error lost: %v", err)
	}
	var e *Error
	if err := NotFoundUnless(nil, false); !errors.As(err, &e) || e.Code != NotFound {
		t.Errorf("missing row = %v", err)
	}
	if err := NotFoundUnless(nil, true); err != nil {
		t.Errorf("found row = %v", err)
	}
}
