package catalog

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Categories handles GET /admin/categories.
func (h *Handler) Categories(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		list, err := h.svc.Categories(r.Context(), sc)
		return map[string]any{"data": list}, err
	})
}

// CreateCategory handles POST /admin/categories.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var in CategoryInput
	routex.Admin(w, r, http.StatusCreated, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.CreateCategory(r.Context(), sc, in)
	})
}

// UpdateCategory handles PATCH /admin/categories/{id}.
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	var in CategoryInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.UpdateCategory(r.Context(), sc, id, in)
	})
}

// Products handles GET /admin/products?q=&cursor=.
func (h *Handler) Products(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		return h.svc.Products(r.Context(), sc, q.Get("q"), q.Get("cursor"))
	})
}

// CreateProduct handles POST /admin/products.
func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var in ProductInput
	routex.Admin(w, r, http.StatusCreated, false, &in, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		return h.svc.CreateProduct(r.Context(), sc, in)
	})
}

// UpdateProduct handles PATCH /admin/products/{id}.
func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	var in ProductInput
	routex.Admin(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.UpdateProduct(r.Context(), sc, id, in)
	})
}

// AddShopProduct handles POST /admin/shops/{id}/products.
func (h *Handler) AddShopProduct(w http.ResponseWriter, r *http.Request) {
	var in ShopProductInput
	routex.Admin(w, r, http.StatusCreated, true, &in, func(r *http.Request, sc db.Scope, shopID uuid.UUID) (any, error) {
		return h.svc.AddShopProduct(r.Context(), sc, shopID, in)
	})
}

// ShopProducts handles GET /console/shops/{shopId}/products?q=&cursor=&available=.
func (h *Handler) ShopProducts(w http.ResponseWriter, r *http.Request) {
	routex.Console[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		available, err := boolParam(q.Get("available"))
		if err != nil {
			return nil, err
		}
		return h.svc.ShopProducts(r.Context(), sc, sc.ShopID, q.Get("q"), q.Get("cursor"), available)
	})
}

// UpdateShopProduct handles PATCH /console/shops/{shopId}/products/{id}.
func (h *Handler) UpdateShopProduct(w http.ResponseWriter, r *http.Request) {
	var in ShopProductPatch
	routex.Console(w, r, http.StatusOK, true, &in, func(r *http.Request, sc db.Scope, id uuid.UUID) (any, error) {
		return h.svc.UpdateShopProduct(r.Context(), sc, sc.ShopID, id, in)
	})
}

// boolParam reads an optional true/false query value; "" means no filter.
func boolParam(s string) (*bool, error) {
	switch s {
	case "":
		return nil, nil
	case "true":
		t := true
		return &t, nil
	case "false":
		f := false
		return &f, nil
	}
	return nil, apperr.Validation(map[string]string{"available": "invalid"})
}
