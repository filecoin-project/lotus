package modules

import (
	"bytes"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"

	cbor "github.com/ipfs/go-ipld-cbor"
	car "github.com/ipld/go-car"
)

func LoadBultinActors(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.UniversalBlockstore) error {
	ctx := helpers.LifecycleCtx(mctx, lc)

	// TODO eventually we want this to start with bundle/manifest CIDs and fetch them from IPFS if
	//      not already loaded.
	//      For now, we just embed the v8 bundle and adjust the manifest CIDs for the migration/actor
	//      metadata.
	blobr := bytes.NewReader(build.BuiltinActorsV8Bundle())
	hdr, err := car.LoadCar(ctx, bs, blobr)
	if err != nil {
		return xerrors.Errorf("error loading builtin actors v8 bundle: %w", err)
	}

	manifestCid := hdr.Roots[0]
	actors.ManifestCids[actors.Version8] = manifestCid

	cborStore := cbor.NewCborStore(bs)
	if err := actors.LoadManifests(ctx, cborStore); err != nil {
		return xerrors.Errorf("error loading actor manifests: %w", err)
	}

	return nil
}
