package bundle

import (
	"context"
	"fmt"
	"os"

	"github.com/ipld/go-car"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/mitchellh/go-homedir"
)

func FetchAndLoadBundleFromRelease(ctx context.Context, basePath string, bs blockstore.Blockstore, av actors.Version, rel, netw string) (cid.Cid, error) {
	fetcher, err := NewBundleFetcher(basePath)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error creating fetcher for builtin-actors version %d: %w", av, err)
	}

	path, err := fetcher.FetchFromRelease(int(av), rel, netw)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error fetching bundle for builtin-actors version %d: %w", av, err)
	}

	return LoadBundle(ctx, bs, path, av)
}

func FetchAndLoadBundleFromURL(ctx context.Context, basePath string, bs blockstore.Blockstore, av actors.Version, rel, netw, url, cksum string) (cid.Cid, error) {
	fetcher, err := NewBundleFetcher(basePath)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error creating fetcher for builtin-actors version %d: %w", av, err)
	}

	path, err := fetcher.FetchFromURL(int(av), rel, netw, url, cksum)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error fetching bundle for builtin-actors version %d: %w", av, err)
	}

	return LoadBundle(ctx, bs, path, av)
}

func LoadBundle(ctx context.Context, bs blockstore.Blockstore, path string, av actors.Version) (cid.Cid, error) {
	f, err := os.Open(path)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error opening bundle for builtin-actors version %d: %w", av, err)
	}
	defer f.Close() //nolint

	hdr, err := car.LoadCar(ctx, bs, f)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error loading builtin actors v%d bundle: %w", av, err)
	}

	// TODO: check that this only has one root?
	manifestCid := hdr.Roots[0]

	if val, ok := build.ActorsCIDs[av]; ok {
		if val != manifestCid {
			return cid.Undef, xerrors.Errorf("actors V%d manifest CID %s did not match CID given in params file: %s", av, manifestCid, val)
		}
	}
	actors.AddManifest(av, manifestCid)

	mfCid, ok := actors.GetManifest(av)
	if !ok {
		return cid.Undef, xerrors.Errorf("missing manifest CID for builtin-actors vrsion %d", av)
	}

	return mfCid, nil
}

// utility for blanket loading outside DI
func FetchAndLoadBundles(ctx context.Context, bs blockstore.Blockstore, bar map[actors.Version]build.Bundle) error {
	netw := build.NetworkBundle

	path := os.Getenv("LOTUS_PATH")
	if path == "" {
		var err error
		path, err = homedir.Expand("~/.lotus")
		if err != nil {
			return err
		}
	}

	for av, bd := range bar {
		envvar := fmt.Sprintf("LOTUS_BUILTIN_ACTORS_V%d_BUNDLE", av)
		switch {
		case os.Getenv(envvar) != "":
			// this is a local bundle, specified by an env var to load from the filesystem
			path := os.Getenv(envvar)

			if _, err := LoadBundle(ctx, bs, path, av); err != nil {
				return err
			}

		case bd.Path[netw] != "":
			if _, err := LoadBundle(ctx, bs, bd.Path[netw], av); err != nil {
				return err
			}

		case bd.URL[netw].URL != "":
			if _, err := FetchAndLoadBundleFromURL(ctx, path, bs, av, bd.Release, netw, bd.URL[netw].URL, bd.URL[netw].Checksum); err != nil {
				return err
			}

		case bd.Release != "":
			if _, err := FetchAndLoadBundleFromRelease(ctx, path, bs, av, bd.Release, netw); err != nil {
				return err
			}
		}
	}

	cborStore := cbor.NewCborStore(bs)
	if err := actors.LoadManifests(ctx, cborStore); err != nil {
		return err
	}

	return nil
}
