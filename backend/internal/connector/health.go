package connector

import (
	"context"

	"gp/internal/shared/db"
)

// Health is the sync summary of the admin's dashboard: how many connectors run,
// how many are failing, and how many rows wait for a decision.
type Health struct {
	Active     int `json:"active"`
	Failing    int `json:"failing"`
	OpenIssues int `json:"openIssues"`
}

// HealthNow counts them. It reads numbers only, never a secret or a source row.
func HealthNow(ctx context.Context, tx *db.Tx) (h Health, err error) {
	err = tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'ACTIVE'), count(*) FILTER (WHERE status = 'FAILING'),
		       (SELECT count(*) FROM sync_issues WHERE status = 'OPEN')
		FROM connectors`).Scan(&h.Active, &h.Failing, &h.OpenIssues)
	return h, err
}
