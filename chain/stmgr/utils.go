package stmgr

import (
	"context"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	amt "github.com/filecoin-project/go-amt-ipld"
	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

func GetMinerWorker(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	recp, err := sm.CallRaw(ctx, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetWorkerAddr,
	}, st, 0)
	if err != nil {
		return address.Undef, xerrors.Errorf("callRaw failed: %w", err)
	}

	if recp.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getting miner worker addr failed (exit code %d)", recp.ExitCode)
	}

	worker, err := address.NewFromBytes(recp.Return)
	if err != nil {
		return address.Undef, err
	}

	if worker.Protocol() == address.ID {
		return address.Undef, xerrors.Errorf("need to resolve worker address to a pubkeyaddr")
	}

	return worker, nil
}

func GetMinerOwner(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	recp, err := sm.CallRaw(ctx, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetOwner,
	}, st, 0)
	if err != nil {
		return address.Undef, xerrors.Errorf("callRaw failed: %w", err)
	}

	if recp.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getting miner owner addr failed (exit code %d)", recp.ExitCode)
	}

	owner, err := address.NewFromBytes(recp.Return)
	if err != nil {
		return address.Undef, err
	}

	if owner.Protocol() == address.ID {
		return address.Undef, xerrors.Errorf("need to resolve owner address to a pubkeyaddr")
	}

	return owner, nil
}

func GetPower(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (types.BigInt, types.BigInt, error) {
	var err error
	enc, err := actors.SerializeParams(&actors.PowerLookupParams{maddr})
	if err != nil {
		return types.EmptyInt, types.EmptyInt, err
	}

	var mpow types.BigInt

	if maddr != address.Undef {
		ret, err := sm.Call(ctx, &types.Message{
			From:   maddr,
			To:     actors.StorageMarketAddress,
			Method: actors.SMAMethods.PowerLookup,
			Params: enc,
		}, ts)
		if err != nil {
			return types.EmptyInt, types.EmptyInt, xerrors.Errorf("failed to get miner power from chain: %w", err)
		}
		if ret.ExitCode != 0 {
			return types.EmptyInt, types.EmptyInt, xerrors.Errorf("failed to get miner power from chain (exit code %d)", ret.ExitCode)
		}

		mpow = types.BigFromBytes(ret.Return)
	}

	ret, err := sm.Call(ctx, &types.Message{
		From:   actors.StorageMarketAddress,
		To:     actors.StorageMarketAddress,
		Method: actors.SMAMethods.GetTotalStorage,
	}, ts)
	if err != nil {
		return types.EmptyInt, types.EmptyInt, xerrors.Errorf("failed to get total power from chain: %w", err)
	}
	if ret.ExitCode != 0 {
		return types.EmptyInt, types.EmptyInt, xerrors.Errorf("failed to get total power from chain (exit code %d)", ret.ExitCode)
	}

	tpow := types.BigFromBytes(ret.Return)

	return mpow, tpow, nil
}

func GetMinerPeerID(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (peer.ID, error) {
	recp, err := sm.Call(ctx, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetPeerID,
	}, ts)
	if err != nil {
		return "", xerrors.Errorf("callRaw failed: %w", err)
	}

	if recp.ExitCode != 0 {
		return "", xerrors.Errorf("getting miner peer ID failed (exit code %d)", recp.ExitCode)
	}

	return peer.IDFromBytes(recp.Return)
}

func GetMinerProvingPeriodEnd(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (uint64, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return 0, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	return mas.ProvingPeriodEnd, nil
}

func GetMinerProvingSet(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) ([]*api.SectorInfo, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	bs := amt.WrapBlockstore(sm.ChainStore().Blockstore())
	a, err := amt.LoadAMT(bs, mas.ProvingSet)
	if err != nil {
		return nil, err
	}

	var sset []*api.SectorInfo
	if err := a.ForEach(func(i uint64, v *cbg.Deferred) error {
		var comms [][]byte
		if err := cbor.DecodeInto(v.Raw, &comms); err != nil {
			return err
		}
		sset = append(sset, &api.SectorInfo{
			SectorID: i,
			CommR:    comms[0],
			CommD:    comms[1],
		})
		return nil
	}); err != nil {
		return nil, err
	}

	return sset, nil
}
