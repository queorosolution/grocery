package cartclient

import (
	"context"
	"errors"
	"net/http"
)

// maxItemsPerBatch is the cart service's limit for POST /v1/items/batch.
const maxItemsPerBatch = 25

// Item is one price list entry. Its id is our shop_products.id; the price is the
// customer price and is for display only: the bill uses our own prices.
type Item struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	UnitPrice   int64  `json:"unitPrice"`
	Active      bool   `json:"active"`
	MaxQuantity int    `json:"maxQuantity,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

// Line is one line of a cart.
type Line struct {
	ID        string  `json:"id"`
	ItemID    *string `json:"itemId"`
	Name      string  `json:"name"`
	UnitPrice int64   `json:"unitPrice"`
	Quantity  int     `json:"quantity"`
	LineTotal int64   `json:"lineTotal"`
}

// Cart is a shopper's cart as the cart service reports it.
type Cart struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"` // "open" or "checked_out"
	Currency  string  `json:"currency"`
	Lines     []Line  `json:"lines"`
	Subtotal  int64   `json:"subtotal"`
	ItemCount int     `json:"itemCount"`
	OrderID   *string `json:"orderId"`
}

// UpsertItems creates or replaces price list items of the tenant, 25 per call.
// It stops at the first failed chunk; saving an item again is harmless, so the
// caller may simply repeat the whole list.
func (c *Client) UpsertItems(ctx context.Context, tenantID string, items []Item) error {
	for len(items) > 0 {
		n := min(maxItemsPerBatch, len(items))
		err := c.do(ctx, call{
			op: "items.batch", method: http.MethodPost, url: c.runtime + "/items/batch", tenant: tenantID,
			body: map[string]any{"items": items[:n]},
		})
		if err != nil {
			return err
		}
		items = items[n:]
	}
	return nil
}

// GetCart reads a cart of the tenant. A cart of another tenant is ErrNotFound.
func (c *Client) GetCart(ctx context.Context, tenantID, cartID string) (Cart, error) {
	var out Cart
	err := c.do(ctx, call{op: "carts.get", method: http.MethodGet, url: path(c.runtime+"/carts", cartID), tenant: tenantID, out: &out})
	return out, err
}

// DeleteCart deletes an open cart (after its bill is submitted). A cart that is
// already gone is not an error: the goal, no cart, is reached.
func (c *Client) DeleteCart(ctx context.Context, tenantID, cartID string) error {
	err := c.do(ctx, call{op: "carts.delete", method: http.MethodDelete, url: path(c.runtime+"/carts", cartID), tenant: tenantID})
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
