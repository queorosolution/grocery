package cartclient

import (
	"context"
	"errors"
	"net/http"
)

const (
	currency = "LKR"
	// tenantPlan is the cart plan of every shop. It must allow more items than
	// the biggest shop has products (open question Q4); the platform operator
	// raises the tenant's limit in the cart service when a shop outgrows it.
	tenantPlan = "pro"
)

// EnsureTenant makes sure the shop has a cart tenant and returns its id and the
// public key (pk_) that the browser uses. It is safe to repeat: if the tenant
// exists already (a first try that died before we saved the result), the
// existing tenant and one of its public keys are used. The secret key of a new
// tenant is not read, kept or logged.
func (c *Client) EnsureTenant(ctx context.Context, externalID, name string) (tenantID, publicKey string, err error) {
	var created struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
		Keys struct {
			Public struct {
				Key string `json:"key"`
			} `json:"public"`
		} `json:"keys"`
	}
	err = c.do(ctx, call{
		op: "tenants.create", method: http.MethodPost, url: c.admin + "/tenants", out: &created,
		body: map[string]any{
			"externalId": externalID, "name": name, "currency": currency, "plan": tenantPlan, "allowedOrigins": c.origins,
		},
	})
	var e *Error
	switch {
	case err == nil:
		return created.Tenant.ID, created.Keys.Public.Key, nil
	case errors.As(err, &e) && errors.Is(err, ErrConflict) && e.TenantID != "":
		key, err := c.publicKeyOf(ctx, e.TenantID)
		return e.TenantID, key, err
	}
	return "", "", err
}

// publicKeyOf returns an active public key of the tenant, making one when it has none.
func (c *Client) publicKeyOf(ctx context.Context, tenantID string) (string, error) {
	var list struct {
		Data []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Key    string `json:"key"`
		} `json:"data"`
	}
	if err := c.do(ctx, call{op: "keys.list", method: http.MethodGet, url: path(c.admin+"/tenants", tenantID) + "/keys", out: &list}); err != nil {
		return "", err
	}
	for _, k := range list.Data {
		if k.Type == "public" && k.Status == "active" && k.Key != "" {
			return k.Key, nil
		}
	}
	var made struct {
		Key string `json:"key"`
	}
	err := c.do(ctx, call{op: "keys.create", method: http.MethodPost, url: path(c.admin+"/tenants", tenantID) + "/keys",
		body: map[string]string{"type": "public"}, out: &made})
	return made.Key, err
}
