package api

import (
	"context"
)

type HelloWorld interface {
	Hello(ctx context.Context) error
}
