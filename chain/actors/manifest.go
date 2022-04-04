package actors

import (
	"context"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/v8/actors/builtin/manifest"
)

var ManifestCids map[Version]cid.Cid = map[Version]cid.Cid{
	// TODO fill in manifest CIDs for v8 and upwards once these are fixed
}

var manifests map[Version]*manifest.Manifest
var actorMeta map[cid.Cid]actorEntry

type actorEntry struct {
	name    string
	version Version
}

func LoadManifests(ctx context.Context, store cbor.IpldStore) error {
	adtStore := adt.WrapStore(ctx, store)

	manifests = make(map[Version]*manifest.Manifest)
	actorMeta = make(map[cid.Cid]actorEntry)

	for av, mfCid := range ManifestCids {
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
	mf, ok := manifests[av]
	if ok {
		return mf.Get(name)
	}

	return cid.Undef, false
}

func GetActorMetaByCode(c cid.Cid) (string, Version, bool) {
	entry, ok := actorMeta[c]
	if !ok {
		return "", -1, false
	}

	return entry.name, entry.version, true
}
