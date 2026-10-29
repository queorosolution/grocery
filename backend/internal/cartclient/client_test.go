package cartclient_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gp/internal/cartclient"
	"gp/internal/shared/apperr"
	"gp/test/fakes"
)

type secretStore map[string]string

func (s secretStore) Get(_ context.Context, name string) (string, error) {
	v, ok := s[name]
	if !ok {
		return "", errors.New("no such secret " + name)
	}
	return v, nil
}

func newClient(t *testing.T) (*cartclient.Client, *fakes.Cart) {
	t.Helper()
	fake := fakes.NewCart(t)
	c := cartclient.New(fake.BaseURL(), secretStore{"cart/platform-key": fakes.CartKey}, []string{"https://www.example.test"}).WithoutDelays()
	return c, fake
}

func items(n int) []cartclient.Item {
	out := make([]cartclient.Item, n)
	for i := range out {
		out[i] = cartclient.Item{ID: fmt.Sprintf("item-%03d", i), Name: fmt.Sprintf("Item %d", i), UnitPrice: int64(1000 + i), Active: true, MaxQuantity: 20}
	}
	return out
}

func TestUpsertItems_SendsChunksOf25(t *testing.T) {
	c, fake := newClient(t)
	tenant, _ := fake.AddTenant("shop-1")
	if err := c.UpsertItems(context.Background(), tenant, items(60)); err != nil {
		t.Fatal(err)
	}
	var sizes []int
	for _, b := range fake.Batches() {
		sizes = append(sizes, len(b.Items))
	}
	if fmt.Sprint(sizes) != "[25 25 10]" || len(fake.Items(tenant)) != 60 {
		t.Fatalf("batch sizes %v, stored %d", sizes, len(fake.Items(tenant)))
	}
	for _, call := range fake.Calls() {
		if call.Tenant != tenant || call.Path != "/v1/items/batch" {
			t.Fatalf("call = %+v", call)
		}
	}
	if err := c.UpsertItems(context.Background(), tenant, nil); err != nil || len(fake.Calls()) != 3 {
		t.Fatalf("an empty list made a call: %v, %d calls", err, len(fake.Calls()))
	}
}

func TestUpsertItems_RetriesServerErrors(t *testing.T) {
	c, fake := newClient(t)
	tenant, _ := fake.AddTenant("shop-1")

	fake.FailNext(2, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "items/batch")
	if err := c.UpsertItems(context.Background(), tenant, items(3)); err != nil {
		t.Fatalf("two failures then success: %v", err)
	}
	if n := len(fake.Calls()); n != 3 {
		t.Fatalf("calls = %d, want 3 (2 failures + 1 success)", n)
	}

	fake.FailNext(1, http.StatusTooManyRequests, "RATE_LIMITED", "items/batch")
	if err := c.UpsertItems(context.Background(), tenant, items(3)); err != nil {
		t.Fatalf("a rate limit is retried: %v", err)
	}

	before := len(fake.Calls())
	fake.FailNext(10, http.StatusBadGateway, "", "items/batch")
	err := c.UpsertItems(context.Background(), tenant, items(3))
	if !errors.Is(err, cartclient.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if n := len(fake.Calls()) - before; n != 3 {
		t.Fatalf("tries = %d, want 3", n)
	}
	var app *apperr.Error
	if got := cartclient.ToAppError(err); !errors.As(got, &app) || app.Code != apperr.CartUnavailable {
		t.Fatalf("app error = %v", got)
	}
}

func TestUpsertItems_RefusalsAreNotRetried(t *testing.T) {
	c, fake := newClient(t)
	tenant, _ := fake.AddTenant("shop-1")
	bad := items(2)
	bad[1].UnitPrice = 1_000_000_000 // over the cart service's limit

	err := c.UpsertItems(context.Background(), tenant, bad)
	if !errors.Is(err, cartclient.ErrRejected) || len(fake.Calls()) != 1 {
		t.Fatalf("err = %v after %d calls", err, len(fake.Calls()))
	}
	if cartclient.ToAppError(err) != err {
		t.Fatal("a refusal must not look like CART_UNAVAILABLE")
	}
	if len(fake.Items(tenant)) != 0 {
		t.Fatal("a refused batch saved items")
	}

	fake.MaxItems = 0
	tenant2, _ := fake.AddTenant("shop-2")
	fake.FailNext(1, http.StatusTooManyRequests, "QUOTA_EXCEEDED", "items/batch")
	err = c.UpsertItems(context.Background(), tenant2, items(1))
	if !errors.Is(err, cartclient.ErrQuota) || len(fake.Calls()) != 2 {
		t.Fatalf("quota: err = %v, calls = %d (a quota answer must not be retried)", err, len(fake.Calls()))
	}
}

func TestClient_UnknownTenantAndWrongKey(t *testing.T) {
	c, fake := newClient(t)
	if err := c.UpsertItems(context.Background(), "tnt_missing", items(1)); !errors.Is(err, cartclient.ErrNotFound) {
		t.Fatalf("unknown tenant: %v", err)
	}
	wrong := cartclient.New(fake.BaseURL(), secretStore{"cart/platform-key": "plk_wrong"}, nil).WithoutDelays()
	tenant, _ := fake.AddTenant("shop-1")
	err := wrong.UpsertItems(context.Background(), tenant, items(1))
	if !errors.Is(err, cartclient.ErrRejected) || strings.Contains(err.Error(), "plk_wrong") {
		t.Fatalf("wrong key: %v", err)
	}
}

func TestClient_NotConfiguredIsUnavailable(t *testing.T) {
	for name, c := range map[string]*cartclient.Client{
		"no base url": cartclient.New("", secretStore{"cart/platform-key": "x"}, nil),
		"no key":      cartclient.New("http://127.0.0.1:1/v1", secretStore{}, nil),
		"empty key":   cartclient.New("http://127.0.0.1:1/v1", secretStore{"cart/platform-key": ""}, nil),
	} {
		err := c.UpsertItems(context.Background(), "tnt", items(1))
		if !errors.Is(err, cartclient.ErrUnavailable) || strings.Contains(err.Error(), "cart/platform-key") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestClient_SlowServiceTimesOut(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(4 * time.Second)
	}))
	defer slow.Close()
	c := cartclient.New(slow.URL+"/v1", secretStore{"cart/platform-key": "plk"}, nil).WithoutDelays()
	start := time.Now()
	err := c.UpsertItems(context.Background(), "tnt", items(1))
	if !errors.Is(err, cartclient.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if took := time.Since(start); took > 12*time.Second {
		t.Fatalf("3 tries of 3 s took %s", took)
	}
}

func TestCarts_GetAndDelete(t *testing.T) {
	c, fake := newClient(t)
	tenant, _ := fake.AddTenant("shop-1")
	other, _ := fake.AddTenant("shop-2")
	fake.PutCart(tenant, cartclient.Cart{ID: "crt_1", Status: "open", Currency: "LKR", Subtotal: 5000, ItemCount: 2,
		Lines: []cartclient.Line{{ID: "ln_1", Name: "Rice", UnitPrice: 2500, Quantity: 2, LineTotal: 5000}}})
	fake.PutCart(tenant, cartclient.Cart{ID: "crt_done", Status: "checked_out"})

	got, err := c.GetCart(context.Background(), tenant, "crt_1")
	if err != nil || got.Subtotal != 5000 || len(got.Lines) != 1 || got.Lines[0].Quantity != 2 {
		t.Fatalf("cart = %+v, %v", got, err)
	}
	if _, err := c.GetCart(context.Background(), other, "crt_1"); !errors.Is(err, cartclient.ErrNotFound) {
		t.Fatalf("a cart of another tenant: %v", err)
	}
	if err := c.DeleteCart(context.Background(), tenant, "crt_1"); err != nil || fake.HasCart(tenant, "crt_1") {
		t.Fatalf("delete: %v", err)
	}
	if err := c.DeleteCart(context.Background(), tenant, "crt_1"); err != nil {
		t.Fatalf("deleting a cart that is gone should be fine: %v", err)
	}
	if err := c.DeleteCart(context.Background(), tenant, "crt_done"); !errors.Is(err, cartclient.ErrRejected) {
		t.Fatalf("a checked out cart: %v", err)
	}
}

func TestEnsureTenant(t *testing.T) {
	c, fake := newClient(t)
	id, key, err := c.EnsureTenant(context.Background(), "shop-1", "Krishna Super")
	if err != nil || id == "" || !strings.HasPrefix(key, "pk_") {
		t.Fatalf("create: %q %q %v", id, key, err)
	}
	if got, _ := fake.TenantFor("shop-1"); got != id {
		t.Fatalf("tenant for the shop = %q", got)
	}

	// The same shop again finds its tenant and its public key; nothing new is made.
	id2, key2, err := c.EnsureTenant(context.Background(), "shop-1", "Krishna Super")
	if err != nil || id2 != id || key2 != key || len(fake.TenantIDs()) != 1 {
		t.Fatalf("repeat: %q %q %v", id2, key2, err)
	}
	if strings.Contains(fmt.Sprint(key, key2), "sk_") {
		t.Fatal("a secret key was returned")
	}
}

func TestEnsureTenant_Failures(t *testing.T) {
	c, fake := newClient(t)
	fake.FailNext(1, http.StatusBadGateway, "", "/admin/v1/tenants")
	if _, _, err := c.EnsureTenant(context.Background(), "shop-1", "Shop"); err != nil {
		t.Fatalf("one 502 is retried: %v", err)
	}
	fake.FailNext(9, http.StatusInternalServerError, "", "/admin/v1/tenants")
	if _, _, err := c.EnsureTenant(context.Background(), "shop-2", "Shop"); !errors.Is(err, cartclient.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := fake.TenantFor("shop-2"); ok {
		t.Fatal("a tenant was made although the service failed")
	}
}
