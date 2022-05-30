package actors

import (
	"context"
	"strings"
	"sync"

	"github.com/filecoin-project/go-state-types/manifest"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var manifestCids map[Version]cid.Cid
var manifests map[Version]*manifest.Manifest
var actorMeta map[cid.Cid]actorEntry

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

func AddManifest(av Version, manifestCid cid.Cid) {
	manifestMx.Lock()
	defer manifestMx.Unlock()

	if manifestCids == nil {
		manifestCids = make(map[Version]cid.Cid)
	}

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

		var actorKeys = GetBuiltinActorsKeys()
		for _, name := range actorKeys {
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
