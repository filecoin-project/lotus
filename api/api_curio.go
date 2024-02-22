package api

import "context"

type Curio interface {
	Version(context.Context) (Version, error) //perm:admin

	// Trigger shutdown
	Shutdown(context.Context) error //perm:admin
}
