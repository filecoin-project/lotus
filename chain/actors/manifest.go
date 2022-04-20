package actors

import (
	"bytes"
	"context"
	"strings"
	"sync"

	"github.com/filecoin-project/go-state-types/manifest"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	car "github.com/ipld/go-car"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var manifestCids map[Version]cid.Cid = map[Version]cid.Cid{
	// TODO fill in manifest CIDs for v8 and upwards once these are fixed
}

var manifests map[Version]*manifest.Manifest
var actorMeta map[cid.Cid]actorEntry

const AccountKey = "account"
const CronKey = "cron"
const InitKey = "init"
const MarketKey = "storagemarket"
const MinerKey = "storageminer"
const MultisigKey = "multisig"
const PaychKey = "paymentchannel"
const PowerKey = "storagepower"
const RewardKey = "reward"
const SystemKey = "system"
const VerifregKey = "verifiedregistry"

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

		for _, name := range []string{
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
		} {
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

func LoadBundle(ctx context.Context, bs blockstore.Blockstore, av Version, data []byte) error {
	blobr := bytes.NewReader(data)

	hdr, err := car.LoadCar(ctx, bs, blobr)
	if err != nil {
		return xerrors.Errorf("error loading builtin actors v%d bundle: %w", av, err)
	}

	// TODO: check that this only has one root?

	manifestCid := hdr.Roots[0]
	AddManifest(av, manifestCid)

	return nil
}
