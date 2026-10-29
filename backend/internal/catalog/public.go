package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shared/validate"
)

// Text search: a word of 2 to 60 characters. The text only ever travels as a
// query parameter, never inside SQL (SQLI-03).
const (
	minQuery, maxQuery = 2, 60
	defaultLimit       = 24
	maxLimit           = 50
	buyAgainLimit      = 20
)

// SearchQuery is a search request after the handler has read the query string.
type SearchQuery struct {
	Q, Shop, Cursor string
	Pin             *geo.Point
}

// Search finds products by name, brand or keywords, typos included. With a shop
// it looks inside that shop; without one it looks in every shop that delivers
// to the pin (every active shop when there is none), and also lists the shops
// whose name contains the text.
func (s *Service) Search(ctx context.Context, in SearchQuery) (SearchResult, error) {
	q := strings.TrimSpace(in.Q)
	after, err := parsePriceCursor(in.Cursor)
	if err == nil {
		err = validate.New().Str("q", q, minQuery, maxQuery).Err()
	}
	out := SearchResult{Shops: []ShopCard{}, Products: PublicPage[ProductCard]{Data: []ProductCard{}}}
	if err != nil {
		return out, err
	}
	err = s.db.Tx(ctx, db.Public(), func(ctx context.Context, tx *db.Tx) error {
		scope, err := s.searchScope(ctx, tx, in, q)
		if err != nil || len(scope.ids) == 0 {
			return err
		}
		out.Shops = scope.matching
		if err := useTypoSearch(ctx, tx); err != nil {
			return err
		}
		rows, err := searchCards(ctx, tx, scope.ids, q, after, defaultLimit)
		if err != nil {
			return err
		}
		out.Products = publicCut(withSlugs(rows, scope.slugs), defaultLimit, func(c ProductCard) fmt.Stringer {
			return priceCursor{Unavailable: !c.Available, Price: c.OnlinePriceCents, ID: c.ShopProductID}
		})
		return nil
	})
	return out, err
}

// searchPlace is where a search looks: the shops' ids, their slugs, and the
// shops whose own name matches the text.
type searchPlace struct {
	ids      []uuid.UUID
	slugs    map[uuid.UUID]string
	matching []ShopCard
}

func (s *Service) searchScope(ctx context.Context, tx *db.Tx, in SearchQuery, q string) (searchPlace, error) {
	p := searchPlace{matching: []ShopCard{}, slugs: map[uuid.UUID]string{}}
	if in.Shop != "" {
		id, found, err := s.shops.IDBySlug(ctx, tx, in.Shop)
		if err != nil || !found {
			return p, apperr.NotFoundUnless(err, found)
		}
		p.ids, p.slugs[id] = []uuid.UUID{id}, in.Shop
		return p, nil
	}
	shops, err := s.visibleShops(ctx, tx, in.Pin, clock.Now())
	if err != nil {
		return p, err
	}
	lower := strings.ToLower(q)
	for _, sh := range shops {
		p.ids, p.slugs[sh.ID] = append(p.ids, sh.ID), sh.Slug
		if strings.Contains(strings.ToLower(sh.Name), lower) {
			p.matching = append(p.matching, sh)
		}
	}
	return p, nil
}

// withSlugs turns rows into cards of a mixed list, each with its shop's slug.
func withSlugs(rows []cardRow, slugs map[uuid.UUID]string) []ProductCard {
	out := make([]ProductCard, len(rows))
	for i, r := range rows {
		out[i] = r.ProductCard
		out[i].ShopSlug = slugs[r.shopID]
	}
	return out
}

// ShopPage is the page of one active shop with its categories and counts.
func (s *Service) ShopPage(ctx context.Context, slug string) (ShopPage, error) {
	var out ShopPage
	err := s.db.Tx(ctx, db.Public(), func(ctx context.Context, tx *db.Tx) error {
		page, found, err := s.shops.PageBySlug(ctx, tx, slug, clock.Now())
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		out.PublicPage = page
		out.Categories, err = categoryCounts(ctx, tx, page.ID)
		return err
	})
	if out.Categories == nil {
		out.Categories = []ShopCategory{}
	}
	return out, err
}

// ProductsQuery is a shop product list request.
type ProductsQuery struct {
	Slug, Category, Q, Cursor string
	Limit                     int
}

// PublicProducts lists an active shop's products by name, one category at a time
// or all, optionally narrowed by a search word.
func (s *Service) PublicProducts(ctx context.Context, in ProductsQuery) (PublicPage[ProductCard], error) {
	q := strings.TrimSpace(in.Q)
	after, err := parseCursor(in.Cursor)
	v := validate.New().Int("limit", int64(in.Limit), 1, maxLimit).Str("category", in.Category, 0, maxCategorySlug)
	if q != "" {
		v.Str("q", q, minQuery, maxQuery)
	}
	if err == nil {
		err = v.Err()
	}
	if err != nil {
		return PublicPage[ProductCard]{}, err
	}
	var out PublicPage[ProductCard]
	err = s.db.Tx(ctx, db.Public(), func(ctx context.Context, tx *db.Tx) error {
		id, found, err := s.shops.IDBySlug(ctx, tx, in.Slug)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if q != "" {
			if err := useTypoSearch(ctx, tx); err != nil {
				return err
			}
		}
		rows, err := shopCards(ctx, tx, id, in.Category, q, after, in.Limit)
		if err != nil {
			return err
		}
		out = publicCut(withSlugs(rows, nil), in.Limit, func(c ProductCard) fmt.Stringer {
			return cursor{Name: c.Name, ID: c.ShopProductID}
		})
		return nil
	})
	return out, err
}

// BuyAgain is what the signed-in customer bought in this shop before, latest
// first, with today's price and whether it is available now.
func (s *Service) BuyAgain(ctx context.Context, scope db.Scope, slug string) ([]ProductCard, error) {
	out := []ProductCard{}
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		id, found, err := s.shops.IDBySlug(ctx, tx, slug)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		rows, err := buyAgainCards(ctx, tx, scope.UserID, id, buyAgainLimit)
		out = append(out, withSlugs(rows, nil)...)
		return err
	})
	return out, err
}

// CategoryTree is the public category tree.
func (s *Service) CategoryTree(ctx context.Context) ([]CategoryNode, error) {
	var out []CategoryNode
	err := s.db.Tx(ctx, db.Public(), func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = publicCategories(ctx, tx)
		return err
	})
	return nonNil(out), err
}
