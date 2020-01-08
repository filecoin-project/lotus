package stmgr

import (
	"context"

	ffi "github.com/filecoin-project/filecoin-ffi"
	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"

	amt "github.com/filecoin-project/go-amt-ipld"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

func GetMinerWorkerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	recp, err := sm.CallRaw(ctx, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetWorkerAddr,
	}, st, nil, 0)
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
	}, st, nil, 0)
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

	var mpow types.BigInt

	if maddr != address.Undef {
		enc, aerr := actors.SerializeParams(&actors.PowerLookupParams{maddr})
		if aerr != nil {
			return types.EmptyInt, types.EmptyInt, aerr
		}
		ret, err := sm.Call(ctx, &types.Message{
			From:   maddr,
			To:     actors.StoragePowerAddress,
			Method: actors.SPAMethods.PowerLookup,
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
		From:   actors.StoragePowerAddress,
		To:     actors.StoragePowerAddress,
		Method: actors.SPAMethods.GetTotalStorage,
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
		return "", xerrors.Errorf("call failed: %w", err)
	}

	if recp.ExitCode != 0 {
		return "", xerrors.Errorf("getting miner peer ID failed (exit code %d)", recp.ExitCode)
	}

	return peer.IDFromBytes(recp.Return)
}

func GetMinerWorker(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (address.Address, error) {
	recp, err := sm.Call(ctx, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetWorkerAddr,
	}, ts)
	if err != nil {
		return address.Undef, xerrors.Errorf("call failed: %w", err)
	}

	if recp.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getting miner peer ID failed (exit code %d)", recp.ExitCode)
	}

	return address.NewFromBytes(recp.Return)
}

func GetMinerElectionPeriodStart(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (uint64, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return 0, xerrors.Errorf("(get eps) failed to load miner actor state: %w", err)
	}

	return mas.ElectionPeriodStart, nil
}

func SectorSetSizes(ctx context.Context, sm *StateManager, maddr address.Address, ts *types.TipSet) (api.MinerSectors, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return api.MinerSectors{}, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	blks := amt.WrapBlockstore(sm.ChainStore().Blockstore())
	ss, err := amt.LoadAMT(blks, mas.Sectors)
	if err != nil {
		return api.MinerSectors{}, err
	}

	ps, err := amt.LoadAMT(blks, mas.ProvingSet)
	if err != nil {
		return api.MinerSectors{}, err
	}

	return api.MinerSectors{
		Pset: ps.Count,
		Sset: ss.Count,
	}, nil
}

func GetMinerProvingSet(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) ([]*api.ChainSectorInfo, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get pset) failed to load miner actor state: %w", err)
	}

	return LoadSectorsFromSet(ctx, sm.ChainStore().Blockstore(), mas.ProvingSet)
}

func GetMinerSectorSet(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) ([]*api.ChainSectorInfo, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return LoadSectorsFromSet(ctx, sm.ChainStore().Blockstore(), mas.Sectors)
}

func GetSectorsForElectionPost(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (*sectorbuilder.SortedPublicSectorInfo, error) {
	sectors, err := GetMinerProvingSet(ctx, sm, ts, maddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to get sector set for miner: %w", err)
	}

	var uselessOtherArray []ffi.PublicSectorInfo
	for _, s := range sectors {
		var uselessBuffer [32]byte
		copy(uselessBuffer[:], s.CommR)
		uselessOtherArray = append(uselessOtherArray, ffi.PublicSectorInfo{
			SectorID: s.SectorID,
			CommR:    uselessBuffer,
		})
	}

	ssi := sectorbuilder.NewSortedPublicSectorInfo(uselessOtherArray)
	return &ssi, nil
}

func GetMinerSectorSize(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (uint64, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return 0, xerrors.Errorf("(get ssize) failed to load miner actor state: %w", err)
	}

	cst := hamt.CSTFromBstore(sm.cs.Blockstore())
	var minfo actors.MinerInfo
	if err := cst.Get(ctx, mas.Info, &minfo); err != nil {
		return 0, xerrors.Errorf("failed to read miner info: %w", err)
	}

	return minfo.SectorSize, nil
}

func GetMinerSlashed(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (uint64, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return 0, xerrors.Errorf("(get mslash) failed to load miner actor state: %w", err)
	}

	return mas.SlashedAt, nil
}

func GetStorageDeal(ctx context.Context, sm *StateManager, dealId uint64, ts *types.TipSet) (*actors.OnChainDeal, error) {
	var state actors.StorageMarketState
	if _, err := sm.LoadActorState(ctx, actors.StorageMarketAddress, &state, ts); err != nil {
		return nil, err
	}

	blks := amt.WrapBlockstore(sm.ChainStore().Blockstore())
	da, err := amt.LoadAMT(blks, state.Deals)
	if err != nil {
		return nil, err
	}

	var ocd actors.OnChainDeal
	if err := da.Get(dealId, &ocd); err != nil {
		return nil, err
	}

	return &ocd, nil
}

func ListMinerActors(ctx context.Context, sm *StateManager, ts *types.TipSet) ([]address.Address, error) {
	var state actors.StoragePowerState
	if _, err := sm.LoadActorState(ctx, actors.StoragePowerAddress, &state, ts); err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(sm.ChainStore().Blockstore())
	miners, err := actors.MinerSetList(ctx, cst, state.Miners)
	if err != nil {
		return nil, err
	}

	return miners, nil
}

func LoadSectorsFromSet(ctx context.Context, bs blockstore.Blockstore, ssc cid.Cid) ([]*api.ChainSectorInfo, error) {
	blks := amt.WrapBlockstore(bs)
	a, err := amt.LoadAMT(blks, ssc)
	if err != nil {
		return nil, err
	}

	var sset []*api.ChainSectorInfo
	if err := a.ForEach(func(i uint64, v *cbg.Deferred) error {
		var comms [][]byte
		if err := cbor.DecodeInto(v.Raw, &comms); err != nil {
			return err
		}
		sset = append(sset, &api.ChainSectorInfo{
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
