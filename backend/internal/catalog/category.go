package catalog

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/validate"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	maxCategorySlug = 60
	defaultWeightG  = 500
	maxWeightG      = 100000
	categoryColumns = `id, parent_id, slug, name, sort, default_weight_g`
)

func (s *Service) Categories(ctx context.Context, scope db.Scope) ([]Category, error) {
	var out []Category
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = listCategories(ctx, tx)
		return err
	})
	return out, err
}

func (s *Service) CreateCategory(ctx context.Context, scope db.Scope, in CategoryInput) (Category, error) {
	c := Category{DefaultWeightG: defaultWeightG}
	c.apply(in)
	var out Category
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := checkCategory(ctx, tx, c); err != nil {
			return err
		}
		var err error
		if out, err = insertCategory(ctx, tx, c); err != nil {
			return categoryClash(err)
		}
		return recordCategory(ctx, tx, scope, "category.create", Category{}, out)
	})
	return out, err
}

func (s *Service) UpdateCategory(ctx context.Context, scope db.Scope, id uuid.UUID, in CategoryInput) (Category, error) {
	var out Category
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := categoryByID(ctx, tx, id)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		c := before
		c.apply(in)
		if err := checkCategory(ctx, tx, c); err != nil {
			return err
		}
		if out, err = saveCategory(ctx, tx, c); err != nil {
			return categoryClash(err)
		}
		return recordCategory(ctx, tx, scope, "category.update", before, out)
	})
	return out, err
}

func (c *Category) apply(in CategoryInput) {
	if in.Slug != nil {
		c.Slug = strings.TrimSpace(*in.Slug)
	}
	if in.Name != nil {
		c.Name = strings.TrimSpace(*in.Name)
	}
	if in.ParentID.Set {
		c.ParentID = in.ParentID.Ptr()
	}
	if in.Sort != nil {
		c.Sort = *in.Sort
	}
	if in.DefaultWeightG != nil {
		c.DefaultWeightG = *in.DefaultWeightG
	}
}

// checkCategory enforces the field limits and the two-level tree: a parent must
// be a top-level category, and a category with children cannot get a parent.
func checkCategory(ctx context.Context, tx *db.Tx, c Category) error {
	v := validate.New().
		Check("slug", slugRe.MatchString(c.Slug) && len(c.Slug) <= maxCategorySlug, "invalid").
		Str("name", c.Name, 1, 60).
		Int("sort", int64(c.Sort), 0, 10000).
		Int("defaultWeightG", int64(c.DefaultWeightG), 1, maxWeightG)
	if err := v.Err(); err != nil || c.ParentID == nil {
		return err
	}
	if *c.ParentID == c.ID {
		return v.Check("parentId", false, "invalid").Err()
	}
	parent, found, err := categoryByID(ctx, tx, *c.ParentID)
	if err != nil {
		return err
	}
	if !found || parent.ParentID != nil {
		return v.Check("parentId", false, "invalid").Err()
	}
	if c.ID != uuid.Nil {
		hasKids, err := hasChildren(ctx, tx, c.ID)
		if err != nil {
			return err
		}
		return v.Check("parentId", !hasKids, "has_children").Err()
	}
	return nil
}

func categoryClash(err error) error {
	if db.IsUniqueViolation(err) {
		return validate.New().Check("slug", false, "taken").Err()
	}
	return err
}

func recordCategory(ctx context.Context, tx *db.Tx, scope db.Scope, action string, before, after Category) error {
	e := audit.Entry{Action: action, Entity: "category", EntityID: after.ID.String(), After: after}
	if before.ID != uuid.Nil {
		e.Before = before
	}
	return audit.Write(ctx, tx, scope, e)
}

func listCategories(ctx context.Context, tx *db.Tx) ([]Category, error) {
	rows, err := tx.Query(ctx, `SELECT `+categoryColumns+` FROM categories ORDER BY sort, name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanCategory(row pgx.Row) (Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.ParentID, &c.Slug, &c.Name, &c.Sort, &c.DefaultWeightG)
	return c, err
}

func categoryByID(ctx context.Context, tx *db.Tx, id uuid.UUID) (Category, bool, error) {
	c, err := scanCategory(tx.QueryRow(ctx, `SELECT `+categoryColumns+` FROM categories WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, false, nil
	}
	return c, err == nil, err
}

func hasChildren(ctx context.Context, tx *db.Tx, id uuid.UUID) (has bool, err error) {
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM categories WHERE parent_id = $1)`, id).Scan(&has)
	return has, err
}

func insertCategory(ctx context.Context, tx *db.Tx, c Category) (Category, error) {
	return scanCategory(tx.QueryRow(ctx, `
		INSERT INTO categories (parent_id, slug, name, sort, default_weight_g) VALUES ($1, $2, $3, $4, $5)
		RETURNING `+categoryColumns, c.ParentID, c.Slug, c.Name, c.Sort, c.DefaultWeightG))
}

func saveCategory(ctx context.Context, tx *db.Tx, c Category) (Category, error) {
	return scanCategory(tx.QueryRow(ctx, `
		UPDATE categories SET parent_id = $2, slug = $3, name = $4, sort = $5, default_weight_g = $6
		WHERE id = $1 RETURNING `+categoryColumns, c.ID, c.ParentID, c.Slug, c.Name, c.Sort, c.DefaultWeightG))
}
