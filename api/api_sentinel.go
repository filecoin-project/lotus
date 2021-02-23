package api

import (
	"context"
)

type Sentinel interface {
	// MethodGroup: Sentinel

	// WatchStart start a watch against the chain
	WatchStart(context.Context) error
}
