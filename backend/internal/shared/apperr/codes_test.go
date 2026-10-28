package apperr

import "testing"

func TestCodes(t *testing.T) {
	if CodeSlotFull.HTTPStatus() != 409 || CodeDeliveryCodeLocked.HTTPStatus() != 423 {
		t.Error("wrong HTTP status")
	}
	if Code("NOPE").Valid() || Code("NOPE").HTTPStatus() != 500 || Code("NOPE").Message() != CodeInternal.Message() {
		t.Error("unknown codes must behave like INTERNAL")
	}
	for _, c := range Codes() {
		if !c.Valid() || c.Message() == "" {
			t.Errorf("code %s is incomplete", c)
		}
	}
}
