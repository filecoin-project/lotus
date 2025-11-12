package actors

import (
	"context"
	"strings"
	"sync"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var manifestCids = make(map[actorstypes.Version]cid.Cid)
var manifests = make(map[actorstypes.Version]map[string]cid.Cid)
var actorMeta = make(map[cid.Cid]actorEntry)

var (
	manifestMx sync.RWMutex
)

type actorEntry struct {
	name    string
	version actorstypes.Version
}

// ClearManifests clears all known manifests. This is usually used in tests that need to switch networks.
func ClearManifests() {
	manifestMx.Lock()
	defer manifestMx.Unlock()

	manifestCids = make(map[actorstypes.Version]cid.Cid)
	manifests = make(map[actorstypes.Version]map[string]cid.Cid)
	actorMeta = make(map[cid.Cid]actorEntry)
}

// RegisterManifest registers an actors manifest with lotus.
func RegisterManifest(av actorstypes.Version, manifestCid cid.Cid, entries map[string]cid.Cid) {
	manifestMx.Lock()
	defer manifestMx.Unlock()

	manifestCids[av] = manifestCid
	manifests[av] = entries

	for name, c := range entries {
		actorMeta[c] = actorEntry{name: name, version: av}
	}
}

func AddActorMeta(name string, codeId cid.Cid, av actorstypes.Version) {
	manifestMx.Lock()
	defer manifestMx.Unlock()
	actorMeta[codeId] = actorEntry{name: name, version: av}
}

// GetManifest gets a loaded manifest.
func GetManifest(av actorstypes.Version) (cid.Cid, bool) {
	manifestMx.RLock()
	defer manifestMx.RUnlock()

	c, ok := manifestCids[av]
	return c, ok
}

// ReadManifest reads a manifest from a blockstore. It does not "add" it.
func ReadManifest(ctx context.Context, store cbor.IpldStore, mfCid cid.Cid) (map[string]cid.Cid, error) {
	adtStore := adt.WrapStore(ctx, store)

	var mf manifest.Manifest
	if err := adtStore.Get(ctx, mfCid, &mf); err != nil {
		return nil, xerrors.Errorf("error reading manifest (cid: %s): %w", mfCid, err)
	}

	if err := mf.Load(ctx, adtStore); err != nil {
		return nil, xerrors.Errorf("error loading manifest (cid: %s): %w", mfCid, err)
	}

	var manifestData manifest.ManifestData
	if err := store.Get(ctx, mf.Data, &manifestData); err != nil {
		return nil, xerrors.Errorf("error loading manifest data: %w", err)
	}

	metadata := make(map[string]cid.Cid)
	for _, entry := range manifestData.Entries {
		metadata[entry.Name] = entry.Code
	}

	return metadata, nil
}

// GetActorCodeIDsFromManifest looks up all builtin actor's code CIDs by actor version for versions that have a manifest.
func GetActorCodeIDsFromManifest(av actorstypes.Version) (map[string]cid.Cid, bool) {
	manifestMx.RLock()
	defer manifestMx.RUnlock()

	cids, ok := manifests[av]
	return cids, ok
}

// LoadManifest will get the manifest for a given Manifest CID from the store and Load data into its entries
func LoadManifest(ctx context.Context, mfCid cid.Cid, adtStore adt.Store) (*manifest.Manifest, error) {
	var mf manifest.Manifest

	if err := adtStore.Get(ctx, mfCid, &mf); err != nil {
		return nil, xerrors.Errorf("error reading manifest: %w", err)
	}

	if err := mf.Load(ctx, adtStore); err != nil {
		return nil, xerrors.Errorf("error loading manifest entries data: %w", err)
	}

	return &mf, nil
}

func GetActorMetaByCode(c cid.Cid) (string, actorstypes.Version, bool) {
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
