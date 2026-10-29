package shop

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// hasSecret is computed from the reference; the reference itself stays inside.
const accountColumns = `id, owner_type, shop_id, provider, mode, merchant_id, status, secret_ref IS NOT NULL, public_info->'bank', created_at, updated_at`

func scanAccount(row pgx.Row) (Account, error) {
	var a Account
	var bank []byte // JSON null (no bank) arrives as SQL NULL
	err := row.Scan(&a.ID, &a.OwnerType, &a.ShopID, &a.Provider, &a.Mode, &a.MerchantID, &a.Status, &a.HasSecret, &bank, &a.CreatedAt, &a.UpdatedAt)
	if err == nil && bank != nil {
		a.Bank = new(Bank)
		err = json.Unmarshal(bank, a.Bank)
	}
	return a, err
}

// publicInfo is the account's public_info column: today only the bank.
func publicInfo(b *Bank) ([]byte, error) {
	if b == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(map[string]*Bank{"bank": b})
}

func listAccounts(ctx context.Context, tx *db.Tx) ([]Account, error) {
	rows, err := tx.Query(ctx, `SELECT `+accountColumns+` FROM payment_accounts ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// insertAccount stores a new account. PayHere accounts are ACTIVE at once (the
// create request carries their credentials); manual ones need none.
func insertAccount(ctx context.Context, tx *db.Tx, id uuid.UUID, in CreateAccountInput, ref, merchant *string) (Account, error) {
	info, err := publicInfo(in.Bank)
	if err != nil {
		return Account{}, err
	}
	return scanAccount(tx.QueryRow(ctx, `
		INSERT INTO payment_accounts (id, owner_type, shop_id, provider, mode, merchant_id, secret_ref, status, public_info)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb) RETURNING `+accountColumns,
		id, in.OwnerType, in.ShopID, in.Provider, in.Mode, merchant, ref, enum.PaymentAccountStatusActive, string(info)))
}

// accountForUpdate locks the row and returns the secret reference separately.
func accountForUpdate(ctx context.Context, tx *db.Tx, id uuid.UUID) (Account, *string, bool, error) {
	var ref *string
	a, err := scanAccount(tx.QueryRow(ctx, `SELECT `+accountColumns+` FROM payment_accounts WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return Account{}, nil, false, db.IgnoreNoRows(err)
	}
	err = tx.QueryRow(ctx, `SELECT secret_ref FROM payment_accounts WHERE id = $1`, id).Scan(&ref)
	return a, ref, true, err
}

func saveAccount(ctx context.Context, tx *db.Tx, a Account, ref *string) (Account, error) {
	info, err := publicInfo(a.Bank)
	if err != nil {
		return Account{}, err
	}
	return scanAccount(tx.QueryRow(ctx, `
		UPDATE payment_accounts SET mode = $2, merchant_id = $3, secret_ref = $4, status = $5, public_info = $6::jsonb
		WHERE id = $1 RETURNING `+accountColumns, a.ID, a.Mode, a.MerchantID, ref, a.Status, string(info)))
}
