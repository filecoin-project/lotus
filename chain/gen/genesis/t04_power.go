package genesis

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func SetupStoragePowerActor(bs bstore.Blockstore) (*types.Actor, error) {
	ctx := context.TODO()
	cst := cbor.NewCborStore(bs)
	nd := hamt.NewNode(cst)
	emptyhamt, err := cst.Put(ctx, nd)
	if err != nil {
		return nil, err
	}

	sms := &power.State{
		TotalNetworkPower:        big.Zero(),
		MinerCount:               0,
		EscrowTable:              emptyhamt,
		CronEventQueue:           emptyhamt,
		PoStDetectedFaultMiners:  emptyhamt,
		Claims:                   emptyhamt,
		NumMinersMeetingMinPower: 0,
	}

	stcid, err := cst.Put(ctx, sms)
	if err != nil {
		return nil, err
	}

	return &types.Actor{
		Code:    actors.StoragePowerCodeCid,
		Head:    stcid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}, nil
}

func adjustStoragePowerTracking(vm *vm.VM, cst cbor.IpldStore, from, to address.Address) error {
	ctx := context.TODO()
	act, err := vm.StateTree().GetActor(actors.StoragePowerAddress)
	if err != nil {
		return xerrors.Errorf("loading storage power actor: %w", err)
	}

	var spst market.State
	if err := cst.Get(ctx, act.Head, &spst); err != nil {
		return xerrors.Errorf("loading storage power actor state: %w", err)
	}

	miners, err := hamt.LoadNode(ctx, cst, spst.Miners)
	if err != nil {
		return xerrors.Errorf("loading miner set: %w", err)
	}

	if err := miners.Delete(ctx, string(from.Bytes())); err != nil {
		return xerrors.Errorf("deleting from spa set: %w", err)
	}

	if err := miners.Set(ctx, string(to.Bytes()), uint64(1)); err != nil {
		return xerrors.Errorf("failed setting miner: %w", err)
	}

	if err := miners.Flush(ctx); err != nil {
		return err
	}

	nminerscid, err := cst.Put(ctx, miners)
	if err != nil {
		return err
	}
	spst.Miners = nminerscid

	nhead, err := cst.Put(ctx, &spst)
	if err != nil {
		return err
	}

	act.Head = nhead

	return nil
}
