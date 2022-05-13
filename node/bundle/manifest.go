package bundle

import (
	"context"
	"io"
	"os"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/mitchellh/go-homedir"
)

func FetchAndLoadBundle(ctx context.Context, basePath string, bs blockstore.Blockstore, av actors.Version, rel, netw string) (cid.Cid, error) {
	fetcher, err := NewBundleFetcher(basePath)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error creating fetcher for builtin-actors version %d: %w", av, err)
	}

	path, err := fetcher.Fetch(int(av), rel, netw)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error fetching bundle for builtin-actors version %d: %w", av, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error opening bundle for builtin-actors vresion %d: %w", av, err)
	}
	defer f.Close() //nolint

	data, err := io.ReadAll(f)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error reading bundle for builtin-actors vresion %d: %w", av, err)
	}

	if err := actors.LoadBundle(ctx, bs, av, data); err != nil {
		return cid.Undef, xerrors.Errorf("error loading bundle for builtin-actors vresion %d: %w", av, err)
	}

	mfCid, ok := actors.GetManifest(av)
	if !ok {
		return cid.Undef, xerrors.Errorf("missing manifest CID for builtin-actors vrsion %d", av)
	}

	return mfCid, nil
}

// utility for blanket loading outside DI
func FetchAndLoadBundles(ctx context.Context, bs blockstore.Blockstore, bar map[actors.Version]string) error {
	netw := build.NetworkBundle
	if netw == "" {
		netw = "mainnet"
	}

	path := os.Getenv("LOTUS_PATH")
	if path == "" {
		var err error
		path, err = homedir.Expand("~/.lotus")
		if err != nil {
			return err
		}
	}

	for av, rel := range bar {
		// if it is prior to v8 and not mainnet, then we don't really have a bundle
		if av < actors.Version8 && netw != "mainnet" {
			log.Warnf("no builtin-actors bundle for %s/v%d", netw, av)
			continue
		}

		if _, err := FetchAndLoadBundle(ctx, path, bs, av, rel, netw); err != nil {
			return err
		}
	}

	cborStore := cbor.NewCborStore(bs)
	if err := actors.LoadManifests(ctx, cborStore); err != nil {
		return err
	}

	return nil
}
