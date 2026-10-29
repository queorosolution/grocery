package catalog

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shared/httpx"
	"gp/internal/shared/routex"
)

// How long Cloudflare may keep each public answer (docs/04 section 4). Nothing
// here depends on who asks, so one cached copy serves everyone.
const (
	homeCacheSeconds       = 60
	searchCacheSeconds     = 30
	shopCacheSeconds       = 60
	productsCacheSeconds   = 60
	categoriesCacheSeconds = 300
)

// servePublic runs a public read and caches the answer. An error is never cached.
func servePublic(w http.ResponseWriter, r *http.Request, maxAge int, read func() (any, error)) {
	out, err := read()
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.CachePublic(w, maxAge)
	httpx.JSON(w, http.StatusOK, out)
}

// Home handles GET /home?lat=&lng=.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	servePublic(w, r, homeCacheSeconds, func() (any, error) {
		pin, err := pinOf(r.URL.Query())
		if err != nil {
			return nil, err
		}
		return h.svc.Home(r.Context(), pin)
	})
}

// Search handles GET /search?q=&shop=&cursor=&lat=&lng=.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	servePublic(w, r, searchCacheSeconds, func() (any, error) {
		q := r.URL.Query()
		pin, err := pinOf(q)
		if err != nil {
			return nil, err
		}
		return h.svc.Search(r.Context(), SearchQuery{Q: q.Get("q"), Shop: q.Get("shop"), Cursor: q.Get("cursor"), Pin: pin})
	})
}

// ShopPage handles GET /shops/{slug}.
func (h *Handler) ShopPage(w http.ResponseWriter, r *http.Request) {
	servePublic(w, r, shopCacheSeconds, func() (any, error) {
		return h.svc.ShopPage(r.Context(), r.PathValue("slug"))
	})
}

// PublicProducts handles GET /shops/{slug}/products?category=&q=&cursor=&limit=.
func (h *Handler) PublicProducts(w http.ResponseWriter, r *http.Request) {
	servePublic(w, r, productsCacheSeconds, func() (any, error) {
		q := r.URL.Query()
		limit, err := limitOf(q.Get("limit"))
		if err != nil {
			return nil, err
		}
		return h.svc.PublicProducts(r.Context(), ProductsQuery{
			Slug: r.PathValue("slug"), Category: q.Get("category"), Q: q.Get("q"), Cursor: q.Get("cursor"), Limit: limit,
		})
	})
}

// CategoryTree handles GET /categories.
func (h *Handler) CategoryTree(w http.ResponseWriter, r *http.Request) {
	servePublic(w, r, categoriesCacheSeconds, func() (any, error) {
		tree, err := h.svc.CategoryTree(r.Context())
		return map[string]any{"data": tree}, err
	})
}

// BuyAgain handles GET /shops/{slug}/buy-again. It is private to the caller and
// is not cached.
func (h *Handler) BuyAgain(w http.ResponseWriter, r *http.Request) {
	routex.User[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		list, err := h.svc.BuyAgain(r.Context(), sc, r.PathValue("slug"))
		return map[string]any{"data": list}, err
	})
}

// pinOf reads lat and lng: both or neither, inside Sri Lanka.
func pinOf(q url.Values) (*geo.Point, error) {
	lat, lng := q.Get("lat"), q.Get("lng")
	if lat == "" && lng == "" {
		return nil, nil
	}
	bad := map[string]string{}
	var p geo.Point
	var err error
	if p.Lat, err = strconv.ParseFloat(lat, 64); err != nil {
		bad["lat"] = reasonFor(lat)
	}
	if p.Lng, err = strconv.ParseFloat(lng, 64); err != nil {
		bad["lng"] = reasonFor(lng)
	}
	if len(bad) == 0 && !p.InSriLanka() {
		bad["lat"], bad["lng"] = "out_of_range", "out_of_range"
	}
	if len(bad) > 0 {
		return nil, apperr.Validation(bad)
	}
	return &p, nil
}

func reasonFor(s string) string {
	if s == "" {
		return "required"
	}
	return "invalid"
}

// limitOf reads the page size: 1 to 50, 24 when left out.
func limitOf(s string) (int, error) {
	if s == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, apperr.Validation(map[string]string{"limit": "invalid"})
	}
	return n, nil
}
