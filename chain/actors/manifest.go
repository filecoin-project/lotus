package actors

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	car "github.com/ipld/go-car"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/node/bundle"
	"github.com/filecoin-project/specs-actors/v8/actors/builtin/manifest"

	"github.com/mitchellh/go-homedir"
)

var manifestCids map[Version]cid.Cid = map[Version]cid.Cid{
	// TODO fill in manifest CIDs for v8 and upwards once these are fixed
}

var manifests map[Version]*manifest.Manifest
var actorMeta map[cid.Cid]actorEntry

var (
	manifestMx sync.RWMutex
)

type actorEntry struct {
	name    string
	version Version
}

func AddManifest(av Version, manifestCid cid.Cid) {
	manifestMx.Lock()
	defer manifestMx.Unlock()

	manifestCids[av] = manifestCid
}

func GetManifest(av Version) (cid.Cid, bool) {
	manifestMx.RLock()
	defer manifestMx.RUnlock()

	c, ok := manifestCids[av]
	return c, ok
}

func LoadManifests(ctx context.Context, store cbor.IpldStore) error {
	manifestMx.Lock()
	defer manifestMx.Unlock()

	return loadManifests(ctx, store)
}

func loadManifests(ctx context.Context, store cbor.IpldStore) error {
	adtStore := adt.WrapStore(ctx, store)

	manifests = make(map[Version]*manifest.Manifest)
	actorMeta = make(map[cid.Cid]actorEntry)

	for av, mfCid := range manifestCids {
		mf := &manifest.Manifest{}
		if err := adtStore.Get(ctx, mfCid, mf); err != nil {
			return xerrors.Errorf("error reading manifest for network version %d (cid: %s): %w", av, mfCid, err)
		}

		if err := mf.Load(ctx, adtStore); err != nil {
			return xerrors.Errorf("error loading manifest for network version %d: %w", av, err)
		}

		manifests[av] = mf

		for _, name := range []string{"system", "init", "cron", "account", "storagepower", "storageminer", "storagemarket", "paymentchannel", "multisig", "reward", "verifiedregistry"} {
			c, ok := mf.Get(name)
			if ok {
				actorMeta[c] = actorEntry{name: name, version: av}
			}
		}
	}

	return nil
}

func GetActorCodeID(av Version, name string) (cid.Cid, bool) {
	manifestMx.RLock()
	defer manifestMx.RUnlock()

	mf, ok := manifests[av]
	if ok {
		return mf.Get(name)
	}

	return cid.Undef, false
}

func GetActorMetaByCode(c cid.Cid) (string, Version, bool) {
	manifestMx.RLock()
	defer manifestMx.RUnlock()

	entry, ok := actorMeta[c]
	if !ok {
		return "", -1, false
	}

	return entry.name, entry.version, true
}

func CanonicalName(name string) string {
	idx := strings.LastIndex(name, "/")
	if idx >= 0 {
		return name[idx+1:]
	}

	return name
}

func FetchAndLoadBundle(ctx context.Context, basePath string, bs blockstore.Blockstore, av Version, rel, netw string) (cid.Cid, error) {
	fetcher, err := bundle.NewBundleFetcher(basePath)
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

	if err := LoadBundle(ctx, bs, av, data); err != nil {
		return cid.Undef, xerrors.Errorf("error loading bundle for builtin-actors vresion %d: %w", av, err)
	}

	mfCid, ok := GetManifest(av)
	if !ok {
		return cid.Undef, xerrors.Errorf("missing manifest CID for builtin-actors vrsion %d", av)
	}

	return mfCid, nil
}

func LoadBundle(ctx context.Context, bs blockstore.Blockstore, av Version, data []byte) error {
	blobr := bytes.NewReader(data)

	hdr, err := car.LoadCar(ctx, bs, blobr)
	if err != nil {
		return xerrors.Errorf("error loading builtin actors v%d bundle: %w", av, err)
	}

	manifestCid := hdr.Roots[0]
	AddManifest(av, manifestCid)

	return nil
}

// utility for blanket loading outside DI
func FetchAndLoadBundles(ctx context.Context, bs blockstore.Blockstore, bar map[Version]string) error {
	// TODO: how to get the network name properly?
	netw := "mainnet"
	if v := os.Getenv("LOTUS_FIL_NETWORK"); v != "" {
		netw = v
	}

	// TODO: how to get the repo properly?
	path, err := homedir.Expand("~/.lotus")
	if err != nil {
		return err
	}

	if p := os.Getenv("LOTUS_PATH"); p != "" {
		path = p
	}

	for av, rel := range bar {
		if _, err := FetchAndLoadBundle(ctx, path, bs, av, rel, netw); err != nil {
			return err
		}
	}

	cborStore := cbor.NewCborStore(bs)
	if err := LoadManifests(ctx, cborStore); err != nil {
		return err
	}

	return nil
}
