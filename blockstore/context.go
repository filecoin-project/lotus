package blockstore

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
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

type CtxWrap struct {
	sub      Blockstore
	wrapFunc func(ctx context.Context) context.Context
}

func NewCtxWrap(sub Blockstore, wf func(ctx context.Context) context.Context) *CtxWrap {
	return &CtxWrap{sub: sub, wrapFunc: wf}
}

func (c *CtxWrap) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	return c.sub.Has(c.wrapFunc(ctx), cid)
}

func (c *CtxWrap) Get(ctx context.Context, cid cid.Cid) (blocks.Block, error) {
	return c.sub.Get(c.wrapFunc(ctx), cid)
}

func (c *CtxWrap) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	return c.sub.GetSize(c.wrapFunc(ctx), cid)
}

func (c *CtxWrap) Put(ctx context.Context, block blocks.Block) error {
	return c.sub.Put(c.wrapFunc(ctx), block)
}

func (c *CtxWrap) PutMany(ctx context.Context, blocks []blocks.Block) error {
	return c.sub.PutMany(c.wrapFunc(ctx), blocks)
}

func (c *CtxWrap) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return c.sub.AllKeysChan(c.wrapFunc(ctx))
}

func (c *CtxWrap) HashOnRead(enabled bool) {
	c.sub.HashOnRead(enabled)
}

func (c *CtxWrap) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error {
	return c.sub.View(c.wrapFunc(ctx), cid, callback)
}

func (c *CtxWrap) DeleteBlock(ctx context.Context, cid cid.Cid) error {
	return c.sub.DeleteBlock(c.wrapFunc(ctx), cid)
}

func (c *CtxWrap) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	return c.sub.DeleteMany(c.wrapFunc(ctx), cids)
}

var _ Blockstore = &CtxWrap{}
