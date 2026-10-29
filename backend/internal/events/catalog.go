package events

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// catalogChunk keeps one event small: a big repricing becomes several events.
const catalogChunk = 200

// AddCatalogChanged announces that these shop products changed (price, stock or
// availability). The ids are sent, never the new values: consumers re-read.
func AddCatalogChanged(ctx context.Context, tx *db.Tx, shopID uuid.UUID, shopProductIDs []uuid.UUID) error {
	for len(shopProductIDs) > 0 {
		n := min(catalogChunk, len(shopProductIDs))
		_, err := Add(ctx, tx, Event{
			Type: enum.EventTypeCatalogChanged, ShopID: shopID, EntityID: shopID.String(),
			Payload: map[string]any{"shopProductIds": shopProductIDs[:n]},
		})
		if err != nil {
			return err
		}
		shopProductIDs = shopProductIDs[n:]
	}
	return nil
}
