package genesis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
)

func SetupInitActor(bs bstore.Blockstore, netname string, initialActors []genesis.Actor) (*types.Actor, error) {
	if len(initialActors) > MaxAccounts {
		return nil, xerrors.New("too many initial actors")
	}

	var ias init_.State
	ias.NextID = MinerStart
	ias.NetworkName = netname

	store := adt.WrapStore(context.TODO(), cbor.NewCborStore(bs))
	amap := adt.MakeEmptyMap(store)

	for i, a := range initialActors {
		if a.Type == genesis.TMultisig {
			continue
		}

		if a.Type != genesis.TAccount {
			return nil, xerrors.Errorf("unsupported account type: %s", a.Type) // TODO: Support msig (skip here)
		}

		var ainfo genesis.AccountMeta
		if err := json.Unmarshal(a.Meta, &ainfo); err != nil {
			return nil, xerrors.Errorf("unmarshaling account meta: %w", err)
		}

		fmt.Printf("init set %s t0%d\n", ainfo.Owner, AccountStart+int64(i))

		value := cbg.CborInt(AccountStart + int64(i))
		if err := amap.Put(adt.AddrKey(ainfo.Owner), &value); err != nil {
			return nil, err
		}
	}

	value := cbg.CborInt(80)
	if err := amap.Put(adt.AddrKey(RootVerifierAddr), &value); err != nil {
		return nil, err
	}

	amapaddr, err := amap.Root()
	if err != nil {
		return nil, err
	}
	ias.AddressMap = amapaddr

	statecid, err := store.Put(store.Context(), &ias)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code: builtin.InitActorCodeID,
		Head: statecid,
	}

	return act, nil
}
