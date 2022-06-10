package bundle

import (
	"context"
	"os"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	car "github.com/ipld/go-car"
)

func LoadBundleData(ctx context.Context, bundle string, bs blockstore.Blockstore) (cid.Cid, error) {
	f, err := os.Open(bundle)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error opening debug bundle: %w", err)
	}
	defer f.Close() //nolint

	hdr, err := car.LoadCar(ctx, bs, f)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error loading debug bundle: %w", err)
	}

	return hdr.Roots[0], nil
}

func LoadManifestData(ctx context.Context, mfCid cid.Cid, bs blockstore.Blockstore) (manifest.ManifestData, error) {
	adtStore := adt.WrapStore(ctx, cbor.NewCborStore(bs))

	var mf manifest.Manifest
	var mfData manifest.ManifestData

	if err := adtStore.Get(ctx, mfCid, &mf); err != nil {
		return mfData, xerrors.Errorf("error reading debug manifest: %w", err)
	}

	if err := mf.Load(ctx, adtStore); err != nil {
		return mfData, xerrors.Errorf("error loading debug manifest: %w", err)
	}

	if err := adtStore.Get(ctx, mf.Data, &mfData); err != nil {
		return mfData, xerrors.Errorf("error fetching manifest data: %w", err)
	}

	return mfData, nil
}
