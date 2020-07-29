package genesis

import (
	"bytes"
	"context"
	"fmt"
	"github.com/filecoin-project/lotus/chain/state"
	"math/rand"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/genesis"
)

func MinerAddress(genesisIndex uint64) address.Address {
	maddr, err := address.NewIDAddress(MinerStart + genesisIndex)
	if err != nil {
		panic(err)
	}

	return maddr
}

type fakedSigSyscalls struct {
	runtime.Syscalls
}

func (fss *fakedSigSyscalls) VerifySignature(signature crypto.Signature, signer address.Address, plaintext []byte) error {
	return nil
}

func mkFakedSigSyscalls(base vm.SyscallBuilder) vm.SyscallBuilder {
	return func(ctx context.Context, cstate *state.StateTree, cst cbor.IpldStore) runtime.Syscalls {
		return &fakedSigSyscalls{
			base(ctx, cstate, cst),
		}
	}
}

func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sroot cid.Cid, miners []genesis.Miner) (cid.Cid, error) {
	vc := func(context.Context, abi.ChainEpoch) (abi.TokenAmount, error) {
		return big.Zero(), nil
	}

	vm, err := vm.NewVM(sroot, 0, &fakeRand{}, cs.Blockstore(), mkFakedSigSyscalls(cs.VMSys()), vc)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create NewVM: %w", err)
	}

	if len(miners) == 0 {
		return cid.Undef, xerrors.New("no genesis miners")
	}

	minerInfos := make([]struct {
		maddr address.Address

		presealExp abi.ChainEpoch

		dealIDs []abi.DealID
	}, len(miners))

	for i, m := range miners {
		// Create miner through power actor
		i := i
		m := m

		spt, err := ffiwrapper.SealProofTypeFromSectorSize(m.SectorSize)
		if err != nil {
			return cid.Undef, err
		}

		{
			constructorParams := &power.CreateMinerParams{
				Owner:         m.Worker,
				Worker:        m.Worker,
				Peer:          []byte(m.PeerId),
				SealProofType: spt,
			}

			params := mustEnc(constructorParams)
			rval, err := doExecValue(ctx, vm, builtin.StoragePowerActorAddr, m.Owner, m.PowerBalance, builtin.MethodsPower.CreateMiner, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}

			var ma power.CreateMinerReturn
			if err := ma.UnmarshalCBOR(bytes.NewReader(rval)); err != nil {
				return cid.Undef, xerrors.Errorf("unmarshaling CreateMinerReturn: %w", err)
			}

			expma := MinerAddress(uint64(i))
			if ma.IDAddress != expma {
				return cid.Undef, xerrors.Errorf("miner assigned wrong address: %s != %s", ma.IDAddress, expma)
			}
			minerInfos[i].maddr = ma.IDAddress

			err = vm.MutateState(ctx, minerInfos[i].maddr, func(cst cbor.IpldStore, st *miner.State) error {
				maxPeriods := miner.MaxSectorExpirationExtension / miner.WPoStProvingPeriod
				minerInfos[i].presealExp = (maxPeriods-1)*miner.WPoStProvingPeriod + st.ProvingPeriodStart - 1

				return nil
			})
		}

		// Add market funds

		{
			params := mustEnc(&minerInfos[i].maddr)
			_, err := doExecValue(ctx, vm, builtin.StorageMarketActorAddr, m.Worker, m.MarketBalance, builtin.MethodsMarket.AddBalance, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}
		}
		{
			params := mustEnc(&m.Worker)
			_, err := doExecValue(ctx, vm, builtin.StorageMarketActorAddr, m.Worker, big.Zero(), builtin.MethodsMarket.AddBalance, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}
		}

		// Publish preseal deals

		{
			publish := func(params *market.PublishStorageDealsParams) error {
				fmt.Printf("publishing %d storage deals on miner %s with worker %s\n", len(params.Deals), params.Deals[0].Proposal.Provider, m.Worker)

				ret, err := doExecValue(ctx, vm, builtin.StorageMarketActorAddr, m.Worker, big.Zero(), builtin.MethodsMarket.PublishStorageDeals, mustEnc(params))
				if err != nil {
					return xerrors.Errorf("failed to create genesis miner: %w", err)
				}
				var ids market.PublishStorageDealsReturn
				if err := ids.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
					return xerrors.Errorf("unmarsahling publishStorageDeals result: %w", err)
				}

				minerInfos[i].dealIDs = append(minerInfos[i].dealIDs, ids.IDs...)
				return nil
			}

			params := &market.PublishStorageDealsParams{}
			for _, preseal := range m.Sectors {
				preseal.Deal.VerifiedDeal = true
				preseal.Deal.EndEpoch = minerInfos[i].presealExp
				params.Deals = append(params.Deals, market.ClientDealProposal{
					Proposal:        preseal.Deal,
					ClientSignature: crypto.Signature{Type: crypto.SigTypeBLS}, // TODO: do we want to sign these? Or do we want to fake signatures for genesis setup?
				})

				if len(params.Deals) == cbg.MaxLength {
					if err := publish(params); err != nil {
						return cid.Undef, err
					}

					params = &market.PublishStorageDealsParams{}
				}
			}

			if len(params.Deals) > 0 {
				if err := publish(params); err != nil {
					return cid.Undef, err
				}
			}
		}
	}

	// adjust total network power for equal pledge per sector
	rawPow, qaPow := big.NewInt(0), big.NewInt(0)
	{
		for i, m := range miners {
			for pi := range m.Sectors {
				rawPow = types.BigAdd(rawPow, types.NewInt(uint64(m.SectorSize)))

				dweight, err := dealWeight(ctx, vm, minerInfos[i].maddr, []abi.DealID{minerInfos[i].dealIDs[pi]}, 0, minerInfos[i].presealExp)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting deal weight: %w", err)
				}

				sectorWeight := miner.QAPowerForWeight(m.SectorSize, minerInfos[i].presealExp, dweight.DealWeight, dweight.VerifiedDealWeight)

				qaPow = types.BigAdd(qaPow, sectorWeight)
			}
		}

		err = vm.MutateState(ctx, builtin.StoragePowerActorAddr, func(cst cbor.IpldStore, st *power.State) error {
			st.TotalQualityAdjPower = qaPow
			st.TotalRawBytePower = rawPow

			st.ThisEpochQualityAdjPower = qaPow
			st.ThisEpochRawBytePower = rawPow
			return nil
		})
		if err != nil {
			return cid.Undef, xerrors.Errorf("mutating state: %w", err)
		}

		err = vm.MutateState(ctx, builtin.RewardActorAddr, func(sct cbor.IpldStore, st *reward.State) error {
			st = reward.ConstructState(qaPow)
			return nil
		})
	}

	for i, m := range miners {
		// Commit sectors
		{
			for pi, preseal := range m.Sectors {
				params := &miner.SectorPreCommitInfo{
					SealProof:     preseal.ProofType,
					SectorNumber:  preseal.SectorID,
					SealedCID:     preseal.CommR,
					SealRandEpoch: -1,
					DealIDs:       []abi.DealID{minerInfos[i].dealIDs[pi]},
					Expiration:    minerInfos[i].presealExp, // TODO: Allow setting externally!
				}

				dweight, err := dealWeight(ctx, vm, minerInfos[i].maddr, params.DealIDs, 0, minerInfos[i].presealExp)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting deal weight: %w", err)
				}

				sectorWeight := miner.QAPowerForWeight(m.SectorSize, minerInfos[i].presealExp, dweight.DealWeight, dweight.VerifiedDealWeight)

				// we've added fake power for this sector above, remove it now
				err = vm.MutateState(ctx, builtin.StoragePowerActorAddr, func(cst cbor.IpldStore, st *power.State) error {
					st.TotalQualityAdjPower = types.BigSub(st.TotalQualityAdjPower, sectorWeight)
					st.TotalRawBytePower = types.BigSub(st.TotalRawBytePower, types.NewInt(uint64(m.SectorSize)))
					return nil
				})
				if err != nil {
					return cid.Undef, xerrors.Errorf("removing fake power: %w", err)
				}

				epochReward, err := currentEpochBlockReward(ctx, vm, minerInfos[i].maddr)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting current epoch reward: %w", err)
				}

				tpow, err := currentTotalPower(ctx, vm, minerInfos[i].maddr)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting current total power: %w", err)
				}

				pledge := miner.InitialPledgeForPower(sectorWeight, tpow.QualityAdjPower, epochReward.ThisEpochBaselinePower, tpow.PledgeCollateral, epochReward.ThisEpochReward, circSupply(ctx, vm, minerInfos[i].maddr))
				fmt.Println(types.FIL(pledge))
				_, err = doExecValue(ctx, vm, minerInfos[i].maddr, m.Worker, pledge, builtin.MethodsMiner.PreCommitSector, mustEnc(params))
				if err != nil {
					return cid.Undef, xerrors.Errorf("failed to confirm presealed sectors: %w", err)
				}

				// Commit one-by-one, otherwise pledge math tends to explode
				confirmParams := &builtin.ConfirmSectorProofsParams{
					Sectors: []abi.SectorNumber{preseal.SectorID},
				}

				_, err = doExecValue(ctx, vm, minerInfos[i].maddr, builtin.StoragePowerActorAddr, big.Zero(), builtin.MethodsMiner.ConfirmSectorProofsValid, mustEnc(confirmParams))
				if err != nil {
					return cid.Undef, xerrors.Errorf("failed to confirm presealed sectors: %w", err)
				}
			}
		}
	}

	// Sanity-check total network power
	err = vm.MutateState(ctx, builtin.StoragePowerActorAddr, func(cst cbor.IpldStore, st *power.State) error {
		if !st.TotalRawBytePower.Equals(rawPow) {
			return xerrors.Errorf("st.TotalRawBytePower doesn't match previously calculated rawPow")
		}

		if !st.TotalQualityAdjPower.Equals(qaPow) {
			return xerrors.Errorf("st.TotalQualityAdjPower doesn't match previously calculated qaPow")
		}

		return nil
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("mutating state: %w", err)
	}

	// TODO: Should we re-ConstructState for the reward actor using rawPow as currRealizedPower here?

	c, err := vm.Flush(ctx)
	if err != nil {
		return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
	}
	return c, nil
}

// TODO: copied from actors test harness, deduplicate or remove from here
type fakeRand struct{}

func (fr *fakeRand) GetRandomness(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch))).Read(out)
	return out, nil
}

func currentTotalPower(ctx context.Context, vm *vm.VM, maddr address.Address) (*power.CurrentTotalPowerReturn, error) {
	pwret, err := doExecValue(ctx, vm, builtin.StoragePowerActorAddr, maddr, big.Zero(), builtin.MethodsPower.CurrentTotalPower, nil)
	if err != nil {
		return nil, err
	}
	var pwr power.CurrentTotalPowerReturn
	if err := pwr.UnmarshalCBOR(bytes.NewReader(pwret)); err != nil {
		return nil, err
	}

	return &pwr, nil
}

func dealWeight(ctx context.Context, vm *vm.VM, maddr address.Address, dealIDs []abi.DealID, sectorStart, sectorExpiry abi.ChainEpoch) (market.VerifyDealsForActivationReturn, error) {
	params := &market.VerifyDealsForActivationParams{
		DealIDs:      dealIDs,
		SectorStart:  sectorStart,
		SectorExpiry: sectorExpiry,
	}

	var dealWeights market.VerifyDealsForActivationReturn
	ret, err := doExecValue(ctx, vm,
		builtin.StorageMarketActorAddr,
		maddr,
		abi.NewTokenAmount(0),
		builtin.MethodsMarket.VerifyDealsForActivation,
		mustEnc(params),
	)
	if err != nil {
		return market.VerifyDealsForActivationReturn{}, err
	}
	if err := dealWeights.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
		return market.VerifyDealsForActivationReturn{}, err
	}

	return dealWeights, nil
}

func currentEpochBlockReward(ctx context.Context, vm *vm.VM, maddr address.Address) (*reward.ThisEpochRewardReturn, error) {
	rwret, err := doExecValue(ctx, vm, builtin.RewardActorAddr, maddr, big.Zero(), builtin.MethodsReward.ThisEpochReward, nil)
	if err != nil {
		return nil, err
	}

	var epochReward reward.ThisEpochRewardReturn
	if err := epochReward.UnmarshalCBOR(bytes.NewReader(rwret)); err != nil {
		return nil, err
	}

	return &epochReward, nil
}

func circSupply(ctx context.Context, vmi *vm.VM, maddr address.Address) abi.TokenAmount {
	unsafeVM := &vm.UnsafeVM{VM: vmi}
	rt := unsafeVM.MakeRuntime(ctx, &types.Message{
		GasLimit: 1_000_000_000,
		From:     maddr,
	}, maddr, 0, 0, 0)

	return rt.TotalFilCircSupply()
}
