package shop_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
	"gp/internal/shop"
	"gp/test/apptest"
)

// fixture is the real API with an admin who passed TOTP.
type fixture struct {
	a *apptest.App
	apptest.AdminAPI
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	a := apptest.New(t)
	return fixture{a: a, AdminAPI: a.AsAdmin()}
}

const newShop = `{"slug":"krishna-super","name":"Krishna Super","phone":"0771234567","addressText":"12 Main Street","lat":6.9271,"lng":79.8612}`

// createShop makes a shop and returns its detail.
func (f fixture) createShop(body string) shop.Detail {
	f.a.T.Helper()
	return apptest.Decode[shop.Detail](f.a.T, f.OK(http.MethodPost, "/admin/shops", body))
}

func (f fixture) shopPath(id uuid.UUID, rest string) string {
	return "/admin/shops/" + id.String() + rest
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// validSettings is a full settings body that passes validation.
func validSettings() shop.Settings {
	return shop.Settings{
		DeliveryType: "SLOT", PaymentProvider: "MANUAL", DriverSource: "SHOP", NotifyChannels: []enum.NotifyChannel{"SMS"},
		MarkupBP: 250, MaxLines: 60, ReviewTimeoutMin: 15, RevisionTimeoutMin: 15, PaymentTimeoutMin: 30,
	}
}

func randomID() uuid.UUID { return uuid.New() }

const allWeek = `{"days":[` +
	`{"weekday":1,"opens":"00:00","closes":"24:00"},{"weekday":2,"opens":"00:00","closes":"24:00"},` +
	`{"weekday":3,"opens":"00:00","closes":"24:00"},{"weekday":4,"opens":"00:00","closes":"24:00"},` +
	`{"weekday":5,"opens":"00:00","closes":"24:00"},{"weekday":6,"opens":"00:00","closes":"24:00"},` +
	`{"weekday":7,"opens":"00:00","closes":"24:00"}]}`

// makeReady creates an ACTIVE shop that meets every readiness rule: open all
// week, a manual payment account, and a bookable slot tomorrow.
func (f fixture) makeReady(slug string) uuid.UUID {
	f.a.T.Helper()
	id := f.createShop(strings.Replace(newShop, "krishna-super", slug, 1)).Shop.ID
	f.OK(http.MethodPatch, f.shopPath(id, ""), `{"status":"ACTIVE"}`)
	f.OK(http.MethodPut, f.shopPath(id, "/hours"), allWeek)
	acc := apptest.Decode[shop.Account](f.a.T, f.OK(http.MethodPost, "/admin/payment-accounts", `{"ownerType":"PLATFORM","provider":"MANUAL"}`))
	s := validSettings()
	s.PaymentAccountID = &acc.ID
	f.OK(http.MethodPut, f.shopPath(id, "/settings"), jsonOf(f.a.T, s))
	f.a.Exec(`INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity)
		VALUES ($1, now() + interval '1 day', now() + interval '1 day 2 hours', now() + interval '20 hours', 10)`, id)
	return id
}
