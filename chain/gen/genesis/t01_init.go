package genesis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/builtin"

	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

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

	cst := cbor.NewCborStore(bs)
	amap := hamt.NewNode(cst, hamt.UseTreeBitWidth(5)) // TODO: use spec adt map

	for i, a := range initialActors {
		if a.Type == genesis.TMultisig {
			addr, _ := address.NewActorAddress(a.Meta)
			fmt.Printf("init set %s t0%d\n", addr, AccountStart+uint64(i))

			if err := amap.Set(context.TODO(), string(addr.Bytes()), AccountStart+uint64(i)); err != nil {
				return nil, err
			}
			continue
		}

		if a.Type != genesis.TAccount {
			return nil, xerrors.Errorf("unsupported account type: %s", a.Type) // TODO: Support msig (skip here)
		}

		var ainfo genesis.AccountMeta
		if err := json.Unmarshal(a.Meta, &ainfo); err != nil {
			return nil, xerrors.Errorf("unmarshaling account meta: %w", err)
		}

		fmt.Printf("init set %s t0%d\n", ainfo.Owner, AccountStart+uint64(i))

		if err := amap.Set(context.TODO(), string(ainfo.Owner.Bytes()), AccountStart+uint64(i)); err != nil {
			return nil, err
		}
	}

	if err := amap.Set(context.TODO(), string(RootVerifierAddr.Bytes()), 80); err != nil {
		return nil, err
	}

	if err := amap.Flush(context.TODO()); err != nil {
		return nil, err
	}
	amapcid, err := cst.Put(context.TODO(), amap)
	if err != nil {
		return nil, err
	}

	ias.AddressMap = amapcid

	statecid, err := cst.Put(context.TODO(), &ias)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code: builtin.InitActorCodeID,
		Head: statecid,
	}

	return act, nil
}
