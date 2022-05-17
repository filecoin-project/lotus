package bundle

import (
	"context"
	"fmt"
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

	data, err := io.ReadAll(f)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error reading bundle for builtin-actors version %d: %w", av, err)
	}

	if err := actors.LoadBundle(ctx, bs, av, data); err != nil {
		return cid.Undef, xerrors.Errorf("error loading bundle for builtin-actors version %d: %w", av, err)
	}

	mfCid, ok := actors.GetManifest(av)
	if !ok {
		return cid.Undef, xerrors.Errorf("missing manifest CID for builtin-actors vrsion %d", av)
	}

	return mfCid, nil
}

// utility for blanket loading outside DI
func FetchAndLoadBundles(ctx context.Context, bs blockstore.Blockstore, bar map[actors.Version]build.Bundle) error {
	netw := build.GetNetworkBundle()

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
