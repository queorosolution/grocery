package authz

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
)

func TestUserScope(t *testing.T) {
	if _, err := UserScope(context.Background()); !apperr.Is(err, apperr.Unauthenticated) {
		t.Errorf("no scope: %v, want UNAUTHENTICATED", err)
	}
	anon := WithScope(context.Background(), db.Public())
	if _, err := UserScope(anon); !apperr.Is(err, apperr.Unauthenticated) {
		t.Errorf("public scope: %v, want UNAUTHENTICATED", err)
	}
	id := uuid.New()
	s, err := UserScope(WithScope(context.Background(), db.Scope{UserID: id}))
	if err != nil || s.UserID != id {
		t.Errorf("user scope: %+v, %v", s, err)
	}
}
