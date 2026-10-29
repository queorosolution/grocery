package fakes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"gp/internal/cartclient"
)

// CartKey is the platform key the fake cart service accepts.
const CartKey = "plk_test_key"

var itemIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Cart is a small fake of the shared cart service: tenants and keys, the price
// list, and carts. It checks the key and X-Tenant-Id like the real one and
// enforces the same limits on items, so a test fails if the client sends
// something the real service would refuse.
type Cart struct {
	srv *httptest.Server

	mu       sync.Mutex
	tenants  map[string]*cartTenant
	byExtID  map[string]string
	failures []failure
	calls    []CartCall
	batches  []Batch
	next     int
	// MaxItems is the plan limit of every tenant created from now on (0 = no limit).
	MaxItems int
}

type cartTenant struct {
	id, externalID string
	keys           []cartKey
	items          map[string]cartclient.Item
	carts          map[string]cartclient.Cart
	maxItems       int
}

type cartKey struct{ typ, key string }

type failure struct {
	left   int
	status int
	code   string
	match  string
}

// CartCall is one request the fake received.
type CartCall struct{ Method, Path, Tenant string }

// Batch is one POST /v1/items/batch the fake accepted.
type Batch struct {
	Tenant string
	Items  []cartclient.Item
}

// NewCart starts the fake and stops it when the test ends.
func NewCart(t *testing.T) *Cart {
	t.Helper()
	c := &Cart{tenants: map[string]*cartTenant{}, byExtID: map[string]string{}}
	c.srv = httptest.NewServer(http.HandlerFunc(c.serve))
	t.Cleanup(c.srv.Close)
	return c
}

// BaseURL is what CART_API_BASE would be (it ends in /v1).
func (c *Cart) BaseURL() string { return c.srv.URL + "/v1" }

// FailNext makes the next n requests whose path contains match answer with the
// status and error code (code may be empty).
func (c *Cart) FailNext(n, status int, code, match string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures = append(c.failures, failure{left: n, status: status, code: code, match: match})
}

// Heal drops the failures that FailNext queued.
func (c *Cart) Heal() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures = nil
}

// Calls lists every request received so far, in order.
func (c *Cart) Calls() []CartCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]CartCall(nil), c.calls...)
}

// Batches lists the accepted item batches, in order.
func (c *Cart) Batches() []Batch {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Batch(nil), c.batches...)
}

// Items is the price list of a tenant.
func (c *Cart) Items(tenantID string) map[string]cartclient.Item {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]cartclient.Item{}
	if t := c.tenants[tenantID]; t != nil {
		for k, v := range t.items {
			out[k] = v
		}
	}
	return out
}

// TenantIDs lists the tenants created so far.
func (c *Cart) TenantIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for id := range c.tenants {
		out = append(out, id)
	}
	return out
}

// TenantFor finds the tenant that was created for an external id.
func (c *Cart) TenantFor(externalID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.byExtID[externalID]
	return id, ok
}

// AddTenant creates a tenant directly, with one public key, as if an earlier
// request had made it. It returns the tenant id and the public key.
func (c *Cart) AddTenant(externalID string) (id, publicKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.newTenant(externalID)
	return t.id, t.keys[0].key
}

// PutCart stores a cart for a tenant.
func (c *Cart) PutCart(tenantID string, cart cartclient.Cart) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tenants[tenantID].carts[cart.ID] = cart
}

// HasCart reports whether the tenant still has the cart.
func (c *Cart) HasCart(tenantID, cartID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.tenants[tenantID].carts[cartID]
	return ok
}

func (c *Cart) newTenant(externalID string) *cartTenant {
	c.next++
	t := &cartTenant{
		id: fmt.Sprintf("tnt_%04d", c.next), externalID: externalID, maxItems: c.MaxItems,
		items: map[string]cartclient.Item{}, carts: map[string]cartclient.Cart{},
		keys: []cartKey{{"public", fmt.Sprintf("pk_test_%04d", c.next)}, {"secret", fmt.Sprintf("sk_test_%04d", c.next)}},
	}
	c.tenants[t.id], c.byExtID[externalID] = t, t.id
	return t
}

func (c *Cart) serve(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, CartCall{r.Method, r.URL.Path, r.Header.Get("X-Tenant-Id")})
	for i := range c.failures {
		if f := &c.failures[i]; f.left > 0 && strings.Contains(r.URL.Path, f.match) {
			f.left--
			reply(w, f.status, f.code, nil)
			return
		}
	}
	if r.Header.Get("Authorization") != "Bearer "+CartKey {
		reply(w, http.StatusUnauthorized, "UNAUTHENTICATED", nil)
		return
	}
	if rest, ok := strings.CutPrefix(r.URL.Path, "/admin/v1/tenants"); ok {
		c.admin(w, r, rest)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/v1/")
	t := c.tenants[r.Header.Get("X-Tenant-Id")]
	switch {
	case !ok:
		reply(w, http.StatusNotFound, "NOT_FOUND", nil)
	case t == nil:
		reply(w, http.StatusNotFound, "TENANT_NOT_FOUND", nil)
	default:
		c.runtime(w, r, t, rest)
	}
}

func (c *Cart) admin(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	switch {
	case rest == "" && r.Method == http.MethodPost:
		var in struct {
			ExternalID, Name, Currency, Plan string
			AllowedOrigins                   []string
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.ExternalID == "" || in.Name == "" || in.Currency != "LKR" || len(in.AllowedOrigins) > 10 {
			reply(w, http.StatusBadRequest, "VALIDATION_ERROR", nil)
			return
		}
		if id, dup := c.byExtID[in.ExternalID]; dup {
			reply(w, http.StatusConflict, "CONFLICT", map[string]any{"tenantId": id})
			return
		}
		t := c.newTenant(in.ExternalID)
		write(w, http.StatusCreated, map[string]any{
			"tenant": map[string]any{"id": t.id},
			"keys":   map[string]any{"public": map[string]any{"id": "key_1", "key": t.keys[0].key}, "secret": map[string]any{"id": "key_2", "key": t.keys[1].key}},
		})
	case len(parts) == 2 && parts[1] == "keys" && c.tenants[parts[0]] != nil:
		c.keys(w, r, c.tenants[parts[0]])
	default:
		reply(w, http.StatusNotFound, "TENANT_NOT_FOUND", nil)
	}
}

func (c *Cart) keys(w http.ResponseWriter, r *http.Request, t *cartTenant) {
	if r.Method == http.MethodPost {
		var in struct{ Type string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		key := fmt.Sprintf("pk_test_new_%d", len(t.keys))
		t.keys = append(t.keys, cartKey{in.Type, key})
		write(w, http.StatusCreated, map[string]any{"id": "key_new", "type": in.Type, "key": key})
		return
	}
	data := []map[string]any{}
	for _, k := range t.keys {
		row := map[string]any{"id": "key", "type": k.typ, "status": "active"}
		if k.typ == "public" { // the real list shows public keys in full and hides the others
			row["key"] = k.key
		}
		data = append(data, row)
	}
	write(w, http.StatusOK, map[string]any{"data": data, "nextCursor": nil})
}

func (c *Cart) runtime(w http.ResponseWriter, r *http.Request, t *cartTenant, rest string) {
	switch {
	case rest == "items/batch" && r.Method == http.MethodPost:
		c.batch(w, r, t)
	case strings.HasPrefix(rest, "carts/") && (r.Method == http.MethodGet || r.Method == http.MethodDelete):
		cart, ok := t.carts[strings.TrimPrefix(rest, "carts/")]
		switch {
		case !ok:
			reply(w, http.StatusNotFound, "CART_NOT_FOUND", nil)
		case r.Method == http.MethodGet:
			write(w, http.StatusOK, cart)
		case cart.Status != "open":
			reply(w, http.StatusConflict, "CART_ALREADY_CHECKED_OUT", nil)
		default:
			delete(t.carts, cart.ID)
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		reply(w, http.StatusNotFound, "NOT_FOUND", nil)
	}
}

// batch saves up to 25 items, all or none, like the real service.
func (c *Cart) batch(w http.ResponseWriter, r *http.Request, t *cartTenant) {
	var in struct{ Items []cartclient.Item }
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || len(in.Items) == 0 || len(in.Items) > 25 {
		reply(w, http.StatusBadRequest, "VALIDATION_ERROR", nil)
		return
	}
	fresh := 0
	for _, it := range in.Items {
		if !itemIDRe.MatchString(it.ID) || it.Name == "" || len(it.Name) > 120 || it.UnitPrice < 0 || it.UnitPrice > 999_999_999 ||
			it.MaxQuantity < 0 || it.MaxQuantity > 999 || len(it.ImageURL) > 500 {
			reply(w, http.StatusBadRequest, "VALIDATION_ERROR", nil)
			return
		}
		if _, have := t.items[it.ID]; !have {
			fresh++
		}
	}
	if t.maxItems > 0 && len(t.items)+fresh > t.maxItems {
		reply(w, http.StatusTooManyRequests, "QUOTA_EXCEEDED", nil)
		return
	}
	for _, it := range in.Items {
		t.items[it.ID] = it
	}
	c.batches = append(c.batches, Batch{Tenant: t.id, Items: in.Items})
	write(w, http.StatusOK, map[string]int{"saved": len(in.Items)})
}

func reply(w http.ResponseWriter, status int, code string, details map[string]any) {
	e := map[string]any{"code": code, "message": "fake", "requestId": "req_fake"}
	if details != nil {
		e["details"] = details
	}
	write(w, status, map[string]any{"error": e})
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
