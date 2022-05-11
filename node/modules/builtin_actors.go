package modules

import (
	"fmt"
	"os"
	"sync"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"

	cid "github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func LoadBultinActors(lc fx.Lifecycle, mctx helpers.MetricsCtx, r repo.LockedRepo, bs dtypes.UniversalBlockstore, ds dtypes.MetadataDS) (result dtypes.BuiltinActorsLoaded, err error) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	// TODO how to properly get the network name?
	//      putting it as a dep in inputs causes a stack overflow in DI from circular dependency
	//      sigh...
	netw := "mainnet"
	if v := os.Getenv("LOTUS_FIL_NETWORK"); v != "" {
		netw = v
	}

	for av, rel := range build.BuiltinActorReleases {
		key := dstore.NewKey(fmt.Sprintf("/builtin-actors/v%d/%s", av, rel))

		data, err := ds.Get(ctx, key)
		switch err {
		case nil:
			mfCid, err := cid.Cast(data)
			if err != nil {
				return result, xerrors.Errorf("error parsing cid for %s: %w", key, err)
			}

			has, err := bs.Has(ctx, mfCid)
			if err != nil {
				return result, xerrors.Errorf("error checking blockstore for manifest cid %s: %w", mfCid, err)
			}

			if has {
				actors.AddManifest(av, mfCid)
				continue
			}

		case dstore.ErrNotFound:

		default:
			return result, xerrors.Errorf("error loading %s from datastore: %w", key, err)
		}

		// ok, we don't have it -- fetch it and add it to the blockstore
		mfCid, err := actors.FetchAndLoadBundle(ctx, r.Path(), bs, av, rel, string(netw))
		if err != nil {
			return result, err
		}

		if err := ds.Put(ctx, key, mfCid.Bytes()); err != nil {
			return result, xerrors.Errorf("error storing manifest CID for builtin-actors vrsion %d to the datastore: %w", av, err)
		}
	}

	cborStore := cbor.NewCborStore(bs)
	if err := actors.LoadManifests(ctx, cborStore); err != nil {
		return result, xerrors.Errorf("error loading actor manifests: %w", err)
	}

	return result, nil
}

// for itests
var testingBundleMx sync.Mutex

func LoadBuiltinActorsTesting(lc fx.Lifecycle, mctx helpers.MetricsCtx, bs dtypes.UniversalBlockstore) (result dtypes.BuiltinActorsLoaded, err error) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	var netw string
	if build.InsecurePoStValidation {
		netw = "testing-fake-proofs"
	} else {
		netw = "testing"
	}

	testingBundleMx.Lock()
	defer testingBundleMx.Unlock()

	for av, rel := range build.BuiltinActorReleases {
		const basePath = "/tmp/lotus-testing"

		if _, err := actors.FetchAndLoadBundle(ctx, basePath, bs, av, rel, netw); err != nil {
			return result, xerrors.Errorf("error loading bundle for builtin-actors vresion %d: %w", av, err)
		}
	}

	cborStore := cbor.NewCborStore(bs)
	if err := actors.LoadManifests(ctx, cborStore); err != nil {
		return result, xerrors.Errorf("error loading actor manifests: %w", err)
	}

	return result, nil
}
