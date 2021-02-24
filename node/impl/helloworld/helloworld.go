package helloworld

import (
	"context"
	"fmt"

	"go.uber.org/fx"
)

type HelloWorldAPI struct {
	fx.In
}

func (h *HelloWorldAPI) Hello(ctx context.Context) error {
	fmt.Println("HELLO WORLD!")
	return nil
}
