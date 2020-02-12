package genesis

import (
	"context"
	"encoding/json"

	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
)

func SetupInitActor(bs bstore.Blockstore, netname string, initialActors []genesis.Actor) (*types.Actor, error) {
	if len(initialActors) > MaxAccounts {
		return nil, xerrors.New("too many initial actors")
	}

	var ias actors.InitActorState
	ias.NextID = MinerStart
	ias.NetworkName = netname

	cst := cbor.NewCborStore(bs)
	amap := hamt.NewNode(cst)

	for i, a := range initialActors {
		if a.Type != genesis.TAccount {
			return nil, xerrors.Errorf("unsupported account type: %s", a.Type) // TODO: Support msig (skip here)
		}

		var ainfo genesis.AccountMeta
		if err := json.Unmarshal(a.Meta, &ainfo); err != nil {
			return nil, xerrors.Errorf("unmarshaling account meta: %w", err)
		}

		if err := amap.Set(context.TODO(), string(ainfo.Owner.Bytes()), AccountStart+uint64(i)); err != nil {
			return nil, err
		}
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
		Code: actors.InitCodeCid,
		Head: statecid,
	}

	return act, nil
}
