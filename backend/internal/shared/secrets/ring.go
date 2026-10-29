package secrets

import (
	"context"

	"gp/internal/shared/crypt"
)

const encKeyName = "enc/key"

// Ring builds the encryption ring from the `enc/key` secret (docs/13 section 2).
// New values are sealed with the given key version. It is read for each use, so a
// rotated key is picked up after the cache time, and the key is never kept here.
func Ring(ctx context.Context, g Getter, version byte) (*crypt.Ring, error) {
	raw, err := g.Get(ctx, encKeyName)
	if err != nil {
		return nil, err
	}
	key, err := crypt.ParseKey(raw)
	if err != nil {
		return nil, err
	}
	return crypt.NewRing(version, map[byte][]byte{version: key})
}
