package cli

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
)

func init() {
	// preload manifest so that we have the correct code CID inventory for cli since that doesn't
	// go through CI
	if len(build.BuiltinActorsV8Bundle()) > 0 {
		bs := blockstore.NewMemory()

		if err := actors.LoadManifestFromBundle(context.TODO(), bs, actors.Version8, build.BuiltinActorsV8Bundle()); err != nil {
			panic(fmt.Errorf("error loading actor manifest: %w", err))
		}
	}
}
