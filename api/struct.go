package api

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
)

// Struct implements API passing calls to user-provided function values.
type Struct struct {
	Internal struct {
		ID      func(context.Context) (peer.ID, error)
		Version func(context.Context) (Version, error)
	}
}

// ID implements API.ID
func (c *Struct) ID(ctx context.Context) (peer.ID, error) {
	return c.Internal.ID(ctx)
}

// Version implements API.Version
func (c *Struct) Version(ctx context.Context) (Version, error) {
	return c.Internal.Version(ctx)
}

var _ API = &Struct{}
