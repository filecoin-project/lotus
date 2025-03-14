package api

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/filecoin-project/go-jsonrpc/auth"

	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/journal/alerting"
)

//                       MODIFYING THE API INTERFACE
//
// When adding / changing methods in this file:
// * Do the change here
// * Adjust implementation in `node/impl/`
// * Run `make gen` - this will:
//  * Generate proxy structs
//  * Generate mocks
//  * Generate markdown docs
//  * Generate openrpc blobs

type Common interface {
	// MethodGroup: Auth

	AuthVerify(ctx context.Context, token string) ([]auth.Permission, error) //perm:read
	AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error)    //perm:admin

	// MethodGroup: Log

	LogList(context.Context) ([]string, error)         //perm:write
	LogSetLevel(context.Context, string, string) error //perm:write

	// LogAlerts returns list of all, active and inactive alerts tracked by the
	// node
	LogAlerts(ctx context.Context) ([]alerting.Alert, error) //perm:admin

	// MethodGroup: Common

	// Version provides information about API provider
	Version(context.Context) (APIVersion, error) //perm:read

	// Discover returns an OpenRPC document describing an RPC API.
	Discover(ctx context.Context) (apitypes.OpenRPCDocument, error) //perm:read

	// trigger graceful shutdown
	Shutdown(context.Context) error //perm:admin

	// StartTime returns node start time
	StartTime(context.Context) (time.Time, error) //perm:read

	// Session returns a random UUID of api provider session
	Session(context.Context) (uuid.UUID, error) //perm:read

	Closing(context.Context) (<-chan struct{}, error) //perm:read
}

// APIVersion provides various build-time information
type APIVersion struct {
	Version string

	// APIVersion is a binary encoded semver version of the remote implementing
	// this api
	//
	// See APIVersion in build/version.go
	APIVersion Version

	// TODO: git commit / os / genesis cid?

	// Seconds
	BlockDelay uint64

	// Agent type, as reported to other nodes, e.g. "lotus"
	Agent string
}

func (v APIVersion) String() string {
	return fmt.Sprintf("%s+api%s", v.Version, v.APIVersion.String())
}
