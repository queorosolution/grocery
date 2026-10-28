package site_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/secrets"
	"gp/internal/site"
	"gp/test/testdb"
)

type fakeSecrets map[string]string

func (f fakeSecrets) Get(_ context.Context, name string) (string, error) {
	if v, ok := f[name]; ok {
		return v, nil
	}
	return "", secrets.ErrNotFound
}

type rig struct {
	env    *testdb.Env
	hook   *httptest.Server
	calls  atomic.Int32
	status atomic.Int32
	reg    *events.Registry
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{env: testdb.New(t)}
	r.status.Store(http.StatusOK)
	r.hook = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.calls.Add(1)
		}
		w.WriteHeader(int(r.status.Load()))
	}))
	t.Cleanup(r.hook.Close)
	r.reg = events.NewRegistry()
	site.NewRebuilderForTest(fakeSecrets{"pages/deploy-hook": r.hook.URL + "/hooks/abc-secret-id"}).Register(r.reg)
	return r
}

// rebuild writes a site.rebuild event the way the shop module does and returns its message.
func (r *rig) rebuild(t *testing.T, at time.Time) events.Message {
	t.Helper()
	var id string
	err := r.env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		id, err = events.Add(ctx, tx, events.Event{Type: enum.EventTypeSiteRebuild})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.env.Owner.Exec(context.Background(), `UPDATE outbox SET created_at = $2 WHERE id = $1`, id, at); err != nil {
		t.Fatal(err)
	}
	return events.Message{ID: id, Type: enum.EventTypeSiteRebuild, At: at}
}

func TestOnRebuild_PostsTheHookOnce(t *testing.T) {
	r := newRig(t)
	m := r.rebuild(t, time.Now())
	for i := 0; i < 2; i++ { // the second is a duplicate delivery
		if err := r.reg.Dispatch(context.Background(), r.env.DB, m); err != nil {
			t.Fatal(err)
		}
	}
	if r.calls.Load() != 1 {
		t.Errorf("hook called %d times, want 1", r.calls.Load())
	}
}

// A burst of edits makes one build: only the newest event deploys (D30).
func TestOnRebuild_OnlyTheNewestEventBuilds(t *testing.T) {
	r := newRig(t)
	now := time.Now()
	first := r.rebuild(t, now.Add(-30*time.Second))
	second := r.rebuild(t, now.Add(-20*time.Second))
	third := r.rebuild(t, now.Add(-10*time.Second))
	for _, m := range []events.Message{first, second, third} {
		if err := r.reg.Dispatch(context.Background(), r.env.DB, m); err != nil {
			t.Fatal(err)
		}
	}
	if r.calls.Load() != 1 {
		t.Errorf("hook called %d times for a burst of 3, want 1", r.calls.Load())
	}
}

func TestOnRebuild_FailureIsRetried(t *testing.T) {
	r := newRig(t)
	m := r.rebuild(t, time.Now())
	r.status.Store(http.StatusInternalServerError)
	err := r.reg.Dispatch(context.Background(), r.env.DB, m)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("a failing hook must be an error: %v", err)
	}
	if strings.Contains(err.Error(), "abc-secret-id") || strings.Contains(err.Error(), r.hook.URL) {
		t.Errorf("the error shows the hook URL: %v", err)
	}
	r.status.Store(http.StatusOK)
	if err := r.reg.Dispatch(context.Background(), r.env.DB, m); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if r.calls.Load() != 2 {
		t.Errorf("hook called %d times, want 2 (failed, then retried)", r.calls.Load())
	}
}

func TestOnRebuild_NetworkErrorHidesTheURL(t *testing.T) {
	r := newRig(t)
	m := r.rebuild(t, time.Now())
	r.hook.Close()
	err := r.reg.Dispatch(context.Background(), r.env.DB, m)
	if err == nil || strings.Contains(err.Error(), "abc-secret-id") {
		t.Errorf("want an error without the hook id, got %v", err)
	}
}

func TestOnRebuild_BadOrMissingHook(t *testing.T) {
	env := testdb.New(t)
	for name, sec := range map[string]fakeSecrets{
		"missing":      {},
		"not a URL":    {"pages/deploy-hook": "%%%"},
		"plain http":   {"pages/deploy-hook": "http://example.com/hook"},
		"no host":      {"pages/deploy-hook": "https:///hook"},
		"wrong scheme": {"pages/deploy-hook": "ftp://example.com/hook"},
	} {
		reg := events.NewRegistry()
		site.NewRebuilder(sec).Register(reg)
		m := events.Message{ID: "evt_" + strings.ToUpper(strings.ReplaceAll(name, " ", "")), Type: enum.EventTypeSiteRebuild}
		if err := reg.Dispatch(context.Background(), env.DB, m); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
