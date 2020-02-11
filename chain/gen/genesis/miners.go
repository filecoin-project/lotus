package genesis

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/genesis"
)

type GenMinerCfg struct {
	PreSeals map[string]genesis.GenesisMiner

	// The addresses of the created miner, this is set by the genesis setup
	MinerAddrs []address.Address

	PeerIDs []peer.ID
}

func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sroot cid.Cid, gmcfg *GenMinerCfg) (cid.Cid, []actors.StorageDealProposal, error) {
	vm, err := vm.NewVM(sroot, 0, nil, actors.NetworkAddress, cs.Blockstore(), cs.VMSys())
	if err != nil {
		return cid.Undef, nil, xerrors.Errorf("failed to create NewVM: %w", err)
	}

	if len(gmcfg.MinerAddrs) == 0 {
		return cid.Undef, nil, xerrors.New("no genesis miners")
	}

	if len(gmcfg.MinerAddrs) != len(gmcfg.PreSeals) {
		return cid.Undef, nil, xerrors.Errorf("miner address list, and preseal count doesn't match (%d != %d)", len(gmcfg.MinerAddrs), len(gmcfg.PreSeals))
	}

	var deals []actors.StorageDealProposal

	for i, maddr := range gmcfg.MinerAddrs {
		ps, psok := gmcfg.PreSeals[maddr.String()]
		if !psok {
			return cid.Undef, nil, xerrors.Errorf("no preseal for miner %s", maddr)
		}

		minerParams := &miner.ConstructorParams{
			OwnerAddr:  ps.Owner,
			WorkerAddr: ps.Worker,
			SectorSize: ps.SectorSize,
			PeerId:     gmcfg.PeerIDs[i], // TODO: grab from preseal too
		}

		params := mustEnc(minerParams)

		// TODO: hardcoding 6500 here is a little fragile, it changes any
		// time anyone changes the initial account allocations
		rval, err := doExecValue(ctx, vm, actors.StoragePowerAddress, ps.Worker, types.FromFil(6500), actors.SPAMethods.CreateMiner, params)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("failed to create genesis miner: %w", err)
		}

		maddrret, err := address.NewFromBytes(rval)
		if err != nil {
			return cid.Undef, nil, err
		}

		_, err = vm.Flush(ctx)
		if err != nil {
			return cid.Undef, nil, err
		}

		cst := cbor.NewCborStore(cs.Blockstore())
		if err := reassignMinerActorAddress(vm, cst, maddrret, maddr); err != nil {
			return cid.Undef, nil, err
		}

		pledgeRequirements := make([]big.Int, len(ps.Sectors))
		for i, sector := range ps.Sectors {
			if sector.Deal.StartEpoch != 0 {
				return cid.Undef, nil, xerrors.New("all deals must start at epoch 0")
			}

			dur := big.NewInt(int64(sector.Deal.Duration()))
			siz := big.NewInt(int64(sector.Deal.PieceSize))
			weight := big.Mul(dur, siz)

			params = mustEnc(&power.OnSectorProveCommitParams{
				Weight: power.SectorStorageWeightDesc{
					SectorSize: ps.SectorSize,
					Duration:   sector.Deal.Duration(),
					DealWeight: weight,
				}})

			ret, err := doExec(ctx, vm, actors.StoragePowerAddress, maddr, builtin.MethodsPower.OnSectorProveCommit, params)
			if err != nil {
				return cid.Undef, nil, xerrors.Errorf("failed to update total storage: %w", err)
			}
			if err := pledgeRequirements[i].UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
				return cid.Undef, nil, xerrors.Errorf("unmarshal pledge requirement: %w", err)
			}
		}

		// we have to flush the vm here because it buffers stuff internally for perf reasons
		if _, err := vm.Flush(ctx); err != nil {
			return cid.Undef, nil, xerrors.Errorf("vm.Flush failed: %w", err)
		}

		st := vm.StateTree()
		mact, err := st.GetActor(maddr)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("get miner actor failed: %w", err)
		}

		var mstate miner.State
		if err := cst.Get(ctx, mact.Head, &mstate); err != nil {
			return cid.Undef, nil, xerrors.Errorf("getting miner actor state failed: %w", err)
		}

		for i, s := range ps.Sectors {
			dur := big.NewInt(int64(s.Deal.Duration()))
			siz := big.NewInt(int64(s.Deal.PieceSize))
			weight := big.Mul(dur, siz)

			oci := miner.SectorOnChainInfo{
				Info: miner.SectorPreCommitInfo{
					SectorNumber: s.SectorID,
					SealedCID:    commcid.ReplicaCommitmentV1ToCID(s.CommR[:]),
					SealEpoch:    0,
					DealIDs:      []abi.DealID{abi.DealID(len(deals))},
					Expiration:   0,
				},
				ActivationEpoch:       s.Deal.StartEpoch,
				DealWeight:            weight,
				PledgeRequirement:     pledgeRequirements[i],
				DeclaredFaultEpoch:    -1,
				DeclaredFaultDuration: -1,
			}

			nssroot := adt.AsArray(cs.Store(ctx), mstate.Sectors)

			if err := nssroot.Set(uint64(s.SectorID), &oci); err != nil {
				return cid.Cid{}, nil, xerrors.Errorf("add sector to set: %w", err)
			}

			mstate.Sectors = nssroot.Root()
			mstate.ProvingSet = nssroot.Root()

			deals = append(deals, s.Deal)
		}

		nstate, err := cst.Put(ctx, &mstate)
		if err != nil {
			return cid.Undef, nil, err
		}

		mact.Head = nstate
		if err := st.SetActor(maddr, mact); err != nil {
			return cid.Undef, nil, err
		}
	}

	c, err := vm.Flush(ctx)
	return c, deals, err
}

func reassignMinerActorAddress(vm *vm.VM, cst cbor.IpldStore, from, to address.Address) error {
	if from == to {
		return nil
	}
	act, err := vm.StateTree().GetActor(from)
	if err != nil {
		return xerrors.Errorf("reassign: failed to get 'from' actor: %w", err)
	}

	_, err = vm.StateTree().GetActor(to)
	if err == nil {
		return xerrors.Errorf("cannot reassign actor, target address taken")
	}
	if err := vm.StateTree().SetActor(to, act); err != nil {
		return xerrors.Errorf("failed to reassign actor: %w", err)
	}

	// TODO: remove from

	if err := adjustStoragePowerTracking(vm, cst, from, to); err != nil {
		return xerrors.Errorf("adjusting storage market tracking: %w", err)
	}

	// Now, adjust the tracking in the init actor
	return initActorReassign(vm, cst, from, to)
}
