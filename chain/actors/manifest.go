package actors

import (
	"context"
	"strings"
	"sync"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var manifestCids map[Version]cid.Cid = make(map[Version]cid.Cid)
var manifests map[Version]map[string]cid.Cid = make(map[Version]map[string]cid.Cid)
var actorMeta map[cid.Cid]actorEntry = make(map[cid.Cid]actorEntry)

const (
	AccountKey  = "account"
	CronKey     = "cron"
	InitKey     = "init"
	MarketKey   = "storagemarket"
	MinerKey    = "storageminer"
	MultisigKey = "multisig"
	PaychKey    = "paymentchannel"
	PowerKey    = "storagepower"
	RewardKey   = "reward"
	SystemKey   = "system"
	VerifregKey = "verifiedregistry"
)

func GetBuiltinActorsKeys() []string {
	return []string{
		AccountKey,
		CronKey,
		InitKey,
		MarketKey,
		MinerKey,
		MultisigKey,
		PaychKey,
		PowerKey,
		RewardKey,
		SystemKey,
		VerifregKey,
	}
}

var (
	manifestMx sync.RWMutex
)

type actorEntry struct {
	name    string
	version Version
}

// ClearManifest clears all known manifests. This is usually used in tests that need to switch networks.
func ClearManifests() {
	manifestMx.Lock()
	defer manifestMx.Unlock()

	manifestCids = make(map[Version]cid.Cid)
	manifests = make(map[Version]map[string]cid.Cid)
	actorMeta = make(map[cid.Cid]actorEntry)
}

// RegisterManifest registers an actors manifest with lotus.
func RegisterManifest(av Version, manifestCid cid.Cid, entries map[string]cid.Cid) {
	manifestMx.Lock()
	defer manifestMx.Unlock()

	manifestCids[av] = manifestCid
	manifests[av] = entries

	for name, c := range entries {
		actorMeta[c] = actorEntry{name: name, version: av}
	}
}

// GetManifest gets a loaded manifest.
func GetManifest(av Version) (cid.Cid, bool) {
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

	actorKeys := GetBuiltinActorsKeys() // TODO: we should be able to enumerate manifests directly.
	metadata := make(map[string]cid.Cid, len(actorKeys))
	for _, name := range actorKeys {
		if c, ok := mf.Get(name); ok {
			metadata[name] = c
		}
	}

	return metadata, nil
}

// GetActorCodeID looks up a builtin actor's code CID by actor version and canonical actor name name.
func GetActorCodeID(av Version, name string) (cid.Cid, bool) {
	manifestMx.RLock()
	defer manifestMx.RUnlock()

	c, ok := manifests[av][name]
	return c, ok
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
