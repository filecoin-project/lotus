package blockstore

import (
	"context"
)

type hotViewKey struct{}

var hotView = hotViewKey{}

// WithHotView constructs a new context with an option that provides a hint to the blockstore
// (e.g. the splitstore) that the object (and its ipld references) should be kept hot.
func WithHotView(ctx context.Context) context.Context {
	return context.WithValue(ctx, hotView, struct{}{})
}

// IsHotView returns true if the hot view option is set in the context
func IsHotView(ctx context.Context) bool {
	v := ctx.Value(hotView)
	return v != nil
}
