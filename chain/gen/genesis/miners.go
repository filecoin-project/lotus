package genesis

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/genesis"
)

func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sroot cid.Cid, miners []genesis.Miner) (cid.Cid, error) {
	vm, err := vm.NewVM(sroot, 0, nil, actors.SystemAddress, cs.Blockstore(), cs.VMSys())
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create NewVM: %w", err)
	}

	if len(miners) == 0 {
		return cid.Undef, xerrors.New("no genesis miners")
	}

	for i, m := range miners {
		// Create miner through power actor

		var maddr address.Address
		{
			constructorParams := &miner.ConstructorParams{
				OwnerAddr:  m.Owner,
				WorkerAddr: m.Worker,
				SectorSize: m.SectorSize,
				PeerId:     m.PeerId,
			}

			params := mustEnc(constructorParams)
			rval, err := doExecValue(ctx, vm, actors.StoragePowerAddress, m.Worker, m.PowerBalance, actors.SPAMethods.CreateMiner, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}

			maddrret, err := address.NewFromBytes(rval)
			if err != nil {
				return cid.Undef, err
			}
			expma, err := address.NewIDAddress(uint64(MinerStart + i))
			if err != nil {
				return cid.Undef, err
			}
			if maddrret != expma {
				return cid.Undef, xerrors.Errorf("miner assigned wrong address: %s != %s", maddrret, expma)
			}
			maddr = maddrret
		}

		// Add market funds

		{
			params := mustEnc(&m.Worker)
			_, err := doExecValue(ctx, vm, actors.StorageMarketAddress, m.Worker, m.MarketBalance, builtin.MethodsMarket.AddBalance, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}
		}

		// Publish preseal deals

		var dealIDs []abi.DealID
		{
			params := &market.PublishStorageDealsParams{}
			for _, preseal := range m.Sectors {
				params.Deals = append(params.Deals, market.ClientDealProposal{
					Proposal:        preseal.Deal,
					ClientSignature: crypto.Signature{},
				})
			}

			ret, err := doExecValue(ctx, vm, builtin.StorageMarketActorAddr, m.Worker, big.Zero(), builtin.MethodsMarket.PublishStorageDeals, mustEnc(params))
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}
			var ids market.PublishStorageDealsReturn
			if err := ids.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
				return cid.Undef, err
			}

			dealIDs = ids.IDs
		}

		// Publish preseals

		{
			for pi, preseal := range m.Sectors {
				// Precommit
				{
					params := &miner.PreCommitSectorParams{Info:miner.SectorPreCommitInfo{
						SectorNumber: preseal.SectorID,
						SealedCID:    commcid.ReplicaCommitmentV1ToCID(preseal.CommR[:]),
						SealEpoch:    0,
						DealIDs:      []abi.DealID{dealIDs[pi]},
						Expiration:   preseal.Deal.EndEpoch,
					}}
					_, err := doExecValue(ctx, vm, maddr, m.Worker, big.Zero(), builtin.MethodsMiner.PreCommitSector, mustEnc(params))
					if err != nil {
						return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
					}
				}

				// Commit
				{
					params := &miner.ProveCommitSectorParams{
						SectorNumber: preseal.SectorID,
						Proof:        abi.SealProof{},
					}
					_, err := doExecValue(ctx, vm, maddr, m.Worker, big.Zero(), builtin.MethodsMiner.ProveCommitSector, mustEnc(params))
					if err != nil {
						return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
					}
				}
			}
		}
	}

	c, err := vm.Flush(ctx)
	return c, err
}
