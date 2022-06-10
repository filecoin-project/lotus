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
	// TODO loading the bundle in every cli invocation adds some latency; we should figure out a way
	//      to load actor CIDs without incurring this hit.
	bs := blockstore.NewMemory()

	if err := bundle.FetchAndLoadBundles(context.Background(), bs, build.BuiltinActorReleases); err != nil {
		panic(err)
	}
}
