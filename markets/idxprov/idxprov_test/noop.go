package idxprov_test

import (
	"context"
)

type NoopMeshCreator struct {
}

func NewNoopMeshCreator() *NoopMeshCreator {
	return &NoopMeshCreator{}
}

func (mc NoopMeshCreator) Connect(ctx context.Context) error {
	return nil
}
