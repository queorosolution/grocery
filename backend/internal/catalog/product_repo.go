package catalog

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
)

const productColumns = `id, barcode, name, brand, size_label, unit, weight_g, category_id, image_url, keywords, status`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.Barcode, &p.Name, &p.Brand, &p.SizeLabel, &p.Unit, &p.WeightG, &p.CategoryID,
		&p.ImageURL, &p.Keywords, &p.Status)
	return p, err
}

// likePattern turns a search word into a LIKE pattern that matches it as plain
// text anywhere: the LIKE wildcards in the word lose their meaning. No word
// gives "", which the queries read as "no filter".
func likePattern(q string) string {
	if q == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
}

// listProducts returns one page plus one more row (to know there is a next page).
func listProducts(ctx context.Context, tx *db.Tx, q string, after *cursor) ([]Product, error) {
	var name *string
	var id uuid.UUID
	if after != nil {
		name, id = &after.Name, after.ID
	}
	rows, err := tx.Query(ctx, `
		SELECT `+productColumns+` FROM products
		WHERE ($1 = '' OR search_doc LIKE lower($1))
		  AND ($2::text IS NULL OR (name, id) > ($2, $3))
		ORDER BY name, id LIMIT $4`, likePattern(q), name, id, pageSize+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func productByID(ctx context.Context, tx *db.Tx, id uuid.UUID) (Product, bool, error) {
	p, err := scanProduct(tx.QueryRow(ctx, `SELECT `+productColumns+` FROM products WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, false, nil
	}
	return p, err == nil, err
}

func insertProduct(ctx context.Context, tx *db.Tx, p Product) (Product, error) {
	return scanProduct(tx.QueryRow(ctx, `
		INSERT INTO products (barcode, name, brand, size_label, unit, weight_g, category_id, image_url, keywords, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+productColumns,
		p.Barcode, p.Name, p.Brand, p.SizeLabel, p.Unit, p.WeightG, p.CategoryID, p.ImageURL, p.Keywords, p.Status))
}

func saveProduct(ctx context.Context, tx *db.Tx, p Product) (Product, error) {
	return scanProduct(tx.QueryRow(ctx, `
		UPDATE products SET barcode = $2, name = $3, brand = $4, size_label = $5, unit = $6, weight_g = $7,
		       category_id = $8, image_url = $9, keywords = $10, status = $11
		WHERE id = $1 RETURNING `+productColumns,
		p.ID, p.Barcode, p.Name, p.Brand, p.SizeLabel, p.Unit, p.WeightG, p.CategoryID, p.ImageURL, p.Keywords, p.Status))
}
