package connector_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"gp/internal/connector"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

func (r *rig) incremental(extra string) connector.Connector {
	r.a.T.Helper()
	c := r.create(r.reader)
	body := `{"sourceTable":"gp_products","moneyUnit":"RUPEES",` + extra + `"columns":{"externalId":"external_id","name":"name","price":"price",
		"barcode":"barcode","mrp":"mrp","stock":"stock","available":"available","updatedAt":"updated_at"}}`
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), body)
	return c
}

func (r *rig) put(ext, name string, price float64, updated string) {
	r.a.Exec(`INSERT INTO gp_products (external_id, name, price, stock, available, updated_at) VALUES ($1, $2, $3, 1, true, $4::timestamptz)
		ON CONFLICT (external_id) DO UPDATE SET price = EXCLUDED.price, updated_at = EXCLUDED.updated_at`, ext, name, price, updated)
}

func (r *rig) runCount(c connector.Connector, status string) int {
	return r.a.Count(`SELECT count(*) FROM connector_runs WHERE connector_id = $1 AND status = $2`, c.ID, status)
}

func TestConnector_IncrementalReadsOnlyWhatChanged(t *testing.T) {
	r := newRig(t)
	seedPreview(r)
	r.put("A1", "Rice", 100, "2026-10-01T10:00:00Z")
	r.put("A2", "Sugar", 100, "2026-10-01T11:00:00Z")
	r.put("A3", "Tea", 100, "2026-10-01T12:00:00Z")
	c := r.incremental("")

	first := r.mustRun(c, enum.RunTriggerManual)
	if first.Read != 3 || first.Changed != 3 {
		t.Fatalf("first run = %+v, want 3 read and 3 changed", first)
	}
	// The cursor is the newest change; the row at that time is read once more, and changes nothing.
	second := r.mustRun(c, enum.RunTriggerManual)
	if second.Read != 1 || second.Changed != 0 {
		t.Errorf("second run = %+v, want only the last row re-read", second)
	}

	r.put("A1", "Rice", 120, "2026-10-02T09:00:00Z")
	third := r.mustRun(c, enum.RunTriggerManual)
	if third.Changed != 1 || r.sold("A1").ShopPrice != 12000 || r.sold("A2").ShopPrice != 10000 {
		t.Errorf("third run = %+v, A1 = %+v", third, r.sold("A1"))
	}
	var cursor time.Time
	if err := r.a.Env.Owner.QueryRow(t.Context(), `SELECT last_cursor FROM connectors WHERE id = $1`, c.ID).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if !cursor.Equal(time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("cursor = %v", cursor)
	}

	// An incremental run never hides products: it only sees what changed.
	r.a.Exec(`DELETE FROM gp_products WHERE external_id = 'A2'`)
	r.mustRun(c, enum.RunTriggerManual)
	if !r.sold("A2").Available {
		t.Error("an incremental run hid a product")
	}

	// A new mapping starts again from the beginning.
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), goodMapping)
	if res := r.mustRun(c, enum.RunTriggerManual); res.Read != 2 {
		t.Errorf("after a new mapping the run read %d rows, want all 2", res.Read)
	}
}

func TestConnector_RunForChosenIDsReadsOnlyThose(t *testing.T) {
	r := newRig(t)
	seedPreview(r)
	r.put("A1", "Rice", 100, "2026-10-01T10:00:00Z")
	r.put("A2", "Sugar", 100, "2026-10-01T11:00:00Z")
	c := r.incremental("")
	r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), "")
	r.mustRun(c, enum.RunTriggerManual)

	// A1 changes in a way the change time does not show: an incremental run misses it...
	r.put("A1", "Rice", 115, "2026-10-01T10:00:00Z")
	r.mustRun(c, enum.RunTriggerManual)
	if r.sold("A1").ShopPrice != 10000 {
		t.Fatal("the incremental run should not have seen A1")
	}
	// ...a run for chosen ids reads it, and does not move the cursor.
	res := r.mustRun(c, enum.RunTriggerWebhook, "A1")
	if res.Read != 1 || r.sold("A1").ShopPrice != 11500 {
		t.Errorf("run by id = %+v, A1 = %+v", res, r.sold("A1"))
	}
}

func TestConnector_RunSkipsWhenItShould(t *testing.T) {
	r := newRig(t)
	seedPreview(r)
	r.put("A1", "Rice", 100, "2026-10-01T10:00:00Z")
	c := r.incremental("")

	// DRAFT: a late schedule or webhook does nothing and writes nothing; "Sync now" works.
	if res := r.mustRun(c, enum.RunTriggerSchedule); res.Status != enum.RunStatusSkipped || r.a.Count(`SELECT count(*) FROM connector_runs`) != 0 {
		t.Errorf("a draft connector ran on a schedule: %+v", res)
	}
	r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), "")

	// Live, but the shop is closed: the schedule does nothing and leaves no row.
	if res := r.mustRun(c, enum.RunTriggerSchedule); res.Status != enum.RunStatusSkipped || r.a.Count(`SELECT count(*) FROM connector_runs`) != 0 {
		t.Errorf("a scheduled run happened outside open hours: %+v", res)
	}
	// A webhook and "Sync now" do not wait for opening hours.
	if res := r.mustRun(c, enum.RunTriggerWebhook); res.Status != enum.RunStatusSuccess {
		t.Errorf("webhook outside hours = %+v", res)
	}
	r.openAlways()
	if res := r.mustRun(c, enum.RunTriggerSchedule); res.Status != enum.RunStatusSuccess {
		t.Errorf("scheduled run in open hours = %+v", res)
	}

	// Paused: only "Sync now" runs.
	r.admin.OK(http.MethodPost, r.connPath(c, "/pause"), "")
	before := r.a.Count(`SELECT count(*) FROM connector_runs`)
	if res := r.mustRun(c, enum.RunTriggerSchedule); res.Status != enum.RunStatusSkipped {
		t.Errorf("paused schedule = %+v", res)
	}
	if r.mustRun(c, enum.RunTriggerManual).Status != enum.RunStatusSuccess || r.a.Count(`SELECT count(*) FROM connector_runs`) != before+1 {
		t.Error("Sync now must run on a paused connector, and only once")
	}
}

func TestConnector_RunsNeverOverlap(t *testing.T) {
	r := newRig(t)
	seedPreview(r)
	r.put("A1", "Rice", 100, "2026-10-01T10:00:00Z")
	c := r.incremental("")
	r.openAlways()
	r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), "")

	r.a.Exec(`INSERT INTO connector_runs (connector_id, trigger) VALUES ($1, 'SCHEDULE')`, c.ID) // a run in progress
	if _, err := r.run(c, enum.RunTriggerManual); !hasCode(err, apperr.ConnectorBusy) {
		t.Errorf("a manual run next to a running one: error = %v, want CONNECTOR_BUSY", err)
	}
	r.admin.Expect(http.MethodPost, r.connPath(c, "/sync"), "", http.StatusConflict, "CONNECTOR_BUSY")
	if res := r.mustRun(c, enum.RunTriggerSchedule); res.Status != enum.RunStatusSkipped || r.runCount(c, "SKIPPED") != 1 {
		t.Errorf("a scheduled run next to a running one = %+v, want one SKIPPED row", res)
	}
	if n := r.runCount(c, "RUNNING"); n != 1 {
		t.Errorf("running rows = %d, want the one that was there", n)
	}

	// A run that was cut off (older than two minutes) stops blocking.
	r.a.Exec(`UPDATE connector_runs SET started_at = $2 WHERE connector_id = $1 AND status = 'RUNNING'`, c.ID, clock.Now().Add(-5*time.Minute))
	if res := r.mustRun(c, enum.RunTriggerManual); res.Status != enum.RunStatusSuccess {
		t.Fatalf("run after a stale one = %+v", res)
	}
	if r.runCount(c, "FAILED") != 1 || r.runCount(c, "RUNNING") != 0 {
		t.Error("the stale run must be marked FAILED")
	}
}

func TestConnector_ThreeFailuresInARowRaiseTheAlert(t *testing.T) {
	r := newRig(t)
	seedPreview(r)
	r.put("A1", "Rice", 100, "2026-10-01T10:00:00Z")
	c := r.incremental("")
	r.openAlways()
	r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), "")
	r.a.Exec(`ALTER ROLE ` + roleOf(r.reader) + ` PASSWORD 'changed-by-the-shop'`) // the shop changed its password

	for i := 1; i <= 4; i++ {
		_, err := r.run(c, enum.RunTriggerManual)
		if !hasCode(err, apperr.ConnectorTestFailed) {
			t.Fatalf("run %d: error = %v, want CONNECTOR_TEST_FAILED", i, err)
		}
		got := apptest.Decode[connector.Connector](t, r.admin.OK(http.MethodGet, r.path("/connector"), ""))
		wantStatus := enum.ConnectorStatusActive
		if i >= 3 {
			wantStatus = enum.ConnectorStatusFailing
		}
		if got.ConsecutiveFailures != i || got.Status != wantStatus {
			t.Errorf("after %d failures: %d failures, status %s", i, got.ConsecutiveFailures, got.Status)
		}
	}
	if n := r.a.Count(`SELECT count(*) FROM outbox WHERE type = 'connector.failed'`); n != 1 {
		t.Errorf("connector.failed events = %d, want exactly 1 (at the third failure)", n)
	}
	if n := r.a.Count(`SELECT count(*) FROM connector_runs WHERE status = 'FAILED' AND error = 'source_failed'`); n != 4 {
		t.Errorf("failed runs with the fixed error = %d, want 4", n)
	}
	runs := r.admin.OK(http.MethodGet, r.connPath(c, "/runs"), "").Body.String()
	r.leaks(t, runs, r.admin.OK(http.MethodGet, r.path("/connector"), "").Body.String())
	if strings.Contains(runs, "changed-by-the-shop") {
		t.Error("the run list shows the password")
	}

	// A good run brings it back.
	r.a.Exec(`ALTER ROLE ` + roleOf(r.reader) + ` PASSWORD '` + sourcePassword + `'`)
	r.mustRun(c, enum.RunTriggerManual)
	got := apptest.Decode[connector.Connector](t, r.admin.OK(http.MethodGet, r.path("/connector"), ""))
	if got.Status != enum.ConnectorStatusActive || got.ConsecutiveFailures != 0 || got.LastSuccessAt == nil {
		t.Errorf("after a good run: %+v", got)
	}
}

func hasCode(err error, code apperr.Code) bool {
	var app *apperr.Error
	return errors.As(err, &app) && app.Code == code
}

func roleOf(connString string) string {
	u, _ := url.Parse(connString)
	return u.User.Username()
}

func TestConnector_ActivatePauseAndTheSchedule(t *testing.T) {
	r := newRig(t)
	c := r.create(r.reader)
	r.admin.Expect(http.MethodPost, r.connPath(c, "/activate"), "", http.StatusConflict, "CONFLICT") // no mapping yet
	r.admin.Expect(http.MethodPost, r.connPath(c, "/pause"), "", http.StatusConflict, "CONFLICT")    // a draft cannot be paused
	r.admin.Expect(http.MethodPost, r.connPath(c, "/sync"), "", http.StatusConflict, "CONFLICT")
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), goodMapping)
	if _, ok := r.a.Schedules.Interval(c.ID); ok {
		t.Fatal("a draft connector has a schedule")
	}

	live := apptest.Decode[connector.Connector](t, r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), ""))
	if n, ok := r.a.Schedules.Interval(c.ID); live.Status != enum.ConnectorStatusActive || !ok || n != 60 {
		t.Errorf("activated: status %s, schedule %d %v", live.Status, n, ok)
	}
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), strings.Replace(goodMapping, `"moneyUnit"`, `"intervalMin":15,"moneyUnit"`, 1))
	if n, _ := r.a.Schedules.Interval(c.ID); n != 15 {
		t.Errorf("the schedule interval = %d after the mapping changed, want 15", n)
	}
	paused := apptest.Decode[connector.Connector](t, r.admin.OK(http.MethodPost, r.connPath(c, "/pause"), ""))
	if _, ok := r.a.Schedules.Interval(c.ID); paused.Status != enum.ConnectorStatusPaused || ok {
		t.Error("pausing must remove the schedule")
	}
	if n := r.a.Count(`SELECT count(*) FROM audit_log WHERE action IN ('connector.activate', 'connector.pause') AND shop_id = $1`, r.shopID); n != 2 {
		t.Errorf("audit rows = %d, want 2", n)
	}

	// "Sync now" answers 202 and starts the run elsewhere.
	w := r.admin.OK(http.MethodPost, r.connPath(c, "/sync"), "")
	jobs := r.a.Invoker.Jobs()
	if w.Code != http.StatusAccepted || len(jobs) != 1 || jobs[0].ConnectorID != c.ID || jobs[0].Trigger != enum.RunTriggerManual {
		t.Errorf("sync now = %d, jobs %+v", w.Code, jobs)
	}
	r.admin.Expect(http.MethodPost, "/admin/connectors/00000000-0000-4000-8000-000000000000/sync", "", http.StatusNotFound, "NOT_FOUND")
	r.admin.Expect(http.MethodGet, "/admin/connectors/00000000-0000-4000-8000-000000000000/runs", "", http.StatusNotFound, "NOT_FOUND")
}
