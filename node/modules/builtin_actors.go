package modules

import (
	"bytes"
	"context"
	"io"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"

	cbor "github.com/ipfs/go-ipld-cbor"
	car "github.com/ipld/go-car"
)

func LoadBultinActors(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.UniversalBlockstore) (result dtypes.BuiltinActorsLoaded, err error) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	// TODO eventually we want this to start with bundle/manifest CIDs and fetch them from IPFS if
	//      not already loaded.
	//      For now, we just embed the v8 bundle and adjust the manifest CIDs for the migration/actor
	//      metadata.
	if len(build.BuiltinActorsV8Bundle()) > 0 {
		blobr := bytes.NewReader(build.BuiltinActorsV8Bundle())
		if err := loadBundle(ctx, actors.Version8, blobr, bs); err != nil {
			return result, err
		}
	}

	// for testing -- need to also set LOTUS_USE_FVM_CUSTOM_BUNDLE=1 to force the fvm to use it.
	if len(build.BuiltinActorsV7Bundle()) > 0 {
		blobr := bytes.NewReader(build.BuiltinActorsV7Bundle())
		if err := loadBundle(ctx, actors.Version7, blobr, bs); err != nil {
			return result, err
		}
	}

	cborStore := cbor.NewCborStore(bs)
	if err := actors.LoadManifests(ctx, cborStore); err != nil {
		return result, xerrors.Errorf("error loading actor manifests: %w", err)
	}

	return result, nil
}

func loadBundle(ctx context.Context, av actors.Version, r io.Reader, bs blockstore.Blockstore) error {
	hdr, err := car.LoadCar(ctx, bs, r)
	if err != nil {
		return xerrors.Errorf("error loading builtin actors v%d bundle: %w", av, err)
	}

	manifestCid := hdr.Roots[0]
	actors.AddManifest(av, manifestCid)

	return nil
}
