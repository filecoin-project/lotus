package bundle

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-car"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("bundle")

func LoadBundleFromFile(ctx context.Context, bs blockstore.Blockstore, path string) (cid.Cid, error) {
	f, err := os.Open(path)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error opening bundle %q for builtin-actors: %w", path, err)
	}
	defer f.Close() //nolint

	return LoadBundle(ctx, bs, f)
}

func LoadBundle(ctx context.Context, bs blockstore.Blockstore, r io.Reader) (cid.Cid, error) {
	hdr, err := car.LoadCar(ctx, bs, r)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error loading builtin actors bundle: %w", err)
	}

	if len(hdr.Roots) != 1 {
		return cid.Undef, xerrors.Errorf("expected one root when loading actors bundle, got %d", len(hdr.Roots))
	}
	return hdr.Roots[0], nil
}

// LoadBundles loads the bundles for the specified actor versions into the passed blockstore, if and
// only if the bundle's manifest is not already present in the blockstore.
func LoadBundles(ctx context.Context, bs blockstore.Blockstore, versions ...actorstypes.Version) error {
	for _, av := range versions {
		// No bundles before version 8.
		if av < actorstypes.Version8 {
			continue
		}

		manifestCid, ok := actors.GetManifest(av)
		if !ok {
			// All manifests are registered on start, so this must succeed.
			return xerrors.Errorf("unknown actor version v%d", av)
		}
		log.Infof("manifest cid: %s", manifestCid)

		if haveManifest, err := bs.Has(ctx, manifestCid); err != nil {
			return xerrors.Errorf("blockstore error when loading manifest %s: %w", manifestCid, err)
		} else if haveManifest {
			// We already have the manifest, and therefore everything under it.
			continue
		}

		var (
			root cid.Cid
			err  error
		)
		if path, ok := build.BundleOverrides[av]; ok {
			root, err = LoadBundleFromFile(ctx, bs, path)
		} else if embedded, ok := build.GetEmbeddedBuiltinActorsBundle(av, buildconstants.NetworkBundle); ok {
			root, err = LoadBundle(ctx, bs, bytes.NewReader(embedded))
		} else {
			err = xerrors.Errorf("bundle for actors version v%d not found", av)
		}

		if err != nil {
			return err
		}

		if root != manifestCid {
			return xerrors.Errorf("expected manifest for actors version %d does not match actual: %s != %s", av, manifestCid, root)
		}
	}

	return nil
}

// LoadGenesisBundle loads the bundle for the actors version the genesis state is based on, if that's
// a wasm bundle version (v8 or later).
// Legacy genesis states (v0 to v7, identity-hashed code CIDs) and code CIDs unknown to this build are
// left alone.
func LoadGenesisBundle(ctx context.Context, bs blockstore.Blockstore, genesis *types.BlockHeader) error {
	st, err := state.LoadStateTree(cbor.NewCborStore(bs), genesis.ParentStateRoot)
	if err != nil {
		return xerrors.Errorf("loading genesis state tree: %w", err)
	}
	sys, err := st.GetActor(builtin.SystemActorAddr)
	if err != nil {
		return xerrors.Errorf("loading genesis system actor: %w", err)
	}

	_, av, ok := actors.GetActorMetaByCode(sys.Code)
	if !ok {
		return nil
	}
	if err := LoadBundles(ctx, bs, av); err != nil {
		return xerrors.Errorf("loading actors v%d bundle for genesis: %w", av, err)
	}
	return nil
}
