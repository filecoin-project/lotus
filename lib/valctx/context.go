package valctx

import (
	"context"
	"time"
)

type Context struct {
	Parent context.Context
}

func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return
}

func (c *Context) Done() <-chan struct{} {
	return nil
}

func (c *Context) Err() error {
	return nil
}

func (c *Context) Value(key interface{}) interface{} {
	return c.Parent.Value(key)
}

var _ context.Context = &Context{}
