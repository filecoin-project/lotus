package cli

import (
	"context"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/bundle"
)

func init() {
	// preload manifest so that we have the correct code CID inventory for cli since that doesn't
	// go through CI
	bs := blockstore.NewMemory()

	if err := bundle.FetchAndLoadBundles(context.Background(), bs, build.BuiltinActorReleases); err != nil {
		panic(err)
	}
}
