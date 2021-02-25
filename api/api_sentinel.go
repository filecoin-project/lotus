package api

import (
	"context"
)

type Sentinel interface {
	// MethodGroup: Sentinel

	// SentinelWatchStart start a watch against the chain
	SentinelWatchStart(context.Context) error
}
