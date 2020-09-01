package genesis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
)

func SetupInitActor(bs bstore.Blockstore, netname string, initialActors []genesis.Actor, rootVerifier genesis.Actor, remainder genesis.Actor) (int64, *types.Actor, map[address.Address]address.Address, error) {
	if len(initialActors) > MaxAccounts {
		return 0, nil, nil, xerrors.New("too many initial actors")
	}

	var ias init_.State
	ias.NextID = MinerStart
	ias.NetworkName = netname

	store := adt.WrapStore(context.TODO(), cbor.NewCborStore(bs))
	amap := adt.MakeEmptyMap(store)

	keyToId := map[address.Address]address.Address{}
	counter := int64(AccountStart)

	for _, a := range initialActors {
		if a.Type == genesis.TMultisig {
			initMultisigActors(a.Meta, keyToId, amap, &counter)
			// Need to add actors for all multisigs too
			continue
		}

		if a.Type != genesis.TAccount {
			return 0, nil, nil, xerrors.Errorf("unsupported account type: %s", a.Type)
		}

		var ainfo genesis.AccountMeta
		if err := json.Unmarshal(a.Meta, &ainfo); err != nil {
			return 0, nil, nil, xerrors.Errorf("unmarshaling account meta: %w", err)
		}

		// It's possible that the key was already added in a multisig
		if _, ok := keyToId[ainfo.Owner]; ok {
			continue
		}

		fmt.Printf("init set %s t0%d\n", ainfo.Owner, counter)

		value := cbg.CborInt(counter)
		if err := amap.Put(adt.AddrKey(ainfo.Owner), &value); err != nil {
			return 0, nil, nil, err
		}
		counter = counter + 1

		var err error
		keyToId[ainfo.Owner], err = address.NewIDAddress(uint64(value))
		if err != nil {
			return 0, nil, nil, err
		}
	}

	if rootVerifier.Type == genesis.TAccount {
		var ainfo genesis.AccountMeta
		if err := json.Unmarshal(rootVerifier.Meta, &ainfo); err != nil {
			return 0, nil, nil, xerrors.Errorf("unmarshaling account meta: %w", err)
		}
		value := cbg.CborInt(80)
		if err := amap.Put(adt.AddrKey(ainfo.Owner), &value); err != nil {
			return 0, nil, nil, err
		}
	} else if rootVerifier.Type == genesis.TMultisig {
		initMultisigActors(rootVerifier.Meta, keyToId, amap, &counter)
	}

	if remainder.Type == genesis.TAccount {
		var ainfo genesis.AccountMeta
		if err := json.Unmarshal(remainder.Meta, &ainfo); err != nil {
			return 0, nil, nil, xerrors.Errorf("unmarshaling account meta: %w", err)
		}
		value := cbg.CborInt(90)
		if err := amap.Put(adt.AddrKey(ainfo.Owner), &value); err != nil {
			return 0, nil, nil, err
		}
	} else if remainder.Type == genesis.TMultisig {
		initMultisigActors(remainder.Meta, keyToId, amap, &counter)
	}

	amapaddr, err := amap.Root()
	if err != nil {
		return 0, nil, nil, err
	}
	ias.AddressMap = amapaddr

	statecid, err := store.Put(store.Context(), &ias)
	if err != nil {
		return 0, nil, nil, err
	}

	act := &types.Actor{
		Code: builtin.InitActorCodeID,
		Head: statecid,
	}

	return counter, act, keyToId, nil
}

func initMultisigActors(meta json.RawMessage, keyToId map[address.Address]address.Address, amap *adt.Map, counter *int64) error {
	var ainfo genesis.MultisigMeta
	if err := json.Unmarshal(meta, &ainfo); err != nil {
		return xerrors.Errorf("unmarshaling account meta: %w", err)
	}
	for _, e := range ainfo.Signers {

		if _, ok := keyToId[e]; ok {
			continue
		}

		fmt.Printf("init set %s t0%d\n", e, *counter)

		value := cbg.CborInt(*counter)
		if err := amap.Put(adt.AddrKey(e), &value); err != nil {
			return err
		}
		*counter = *counter + 1
		var err error
		keyToId[e], err = address.NewIDAddress(uint64(value))
		if err != nil {
			return err
		}

	}

	return nil

}
