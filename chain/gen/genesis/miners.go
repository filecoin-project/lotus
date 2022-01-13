package genesis

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"

	runtime7 "github.com/filecoin-project/specs-actors/v7/actors/runtime"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	power0 "github.com/filecoin-project/specs-actors/actors/builtin/power"
	reward0 "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	reward2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/reward"
	market4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/market"
	power4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/power"
	reward4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/reward"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
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
	runtime7.Syscalls
}

func (fss *fakedSigSyscalls) VerifySignature(signature crypto.Signature, signer address.Address, plaintext []byte) error {
	return nil
}

func mkFakedSigSyscalls(base vm.SyscallBuilder) vm.SyscallBuilder {
	return func(ctx context.Context, rt *vm.Runtime) runtime7.Syscalls {
		return &fakedSigSyscalls{
			base(ctx, rt),
		}
	}
}

// Note: Much of this is brittle, if the methodNum / param / return changes, it will break things
func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sys vm.SyscallBuilder, sroot cid.Cid, miners []genesis.Miner, nv network.Version) (cid.Cid, error) {

	cst := cbor.NewCborStore(cs.StateBlockstore())
	av, err := actors.VersionForNetwork(nv)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get network version: %w", err)
	}

	csc := func(context.Context, abi.ChainEpoch, *state.StateTree) (abi.TokenAmount, error) {
		return big.Zero(), nil
	}

	vmopt := &vm.VMOpts{
		StateBase:      sroot,
		Epoch:          0,
		Rand:           &fakeRand{},
		Bstore:         cs.StateBlockstore(),
		Actors:         filcns.NewActorRegistry(),
		Syscalls:       mkFakedSigSyscalls(sys),
		CircSupplyCalc: csc,
		NtwkVersion: func(_ context.Context, _ abi.ChainEpoch) network.Version {
			return nv
		},
		BaseFee: types.NewInt(0),
	}

	vm, err := vm.NewVM(ctx, vmopt)
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

	maxPeriods := policy.GetMaxSectorExpirationExtension() / miner.WPoStProvingPeriod
	for i, m := range miners {
		// Create miner through power actor
		i := i
		m := m

		spt, err := miner.SealProofTypeFromSectorSize(m.SectorSize, nv)
		if err != nil {
			return cid.Undef, err
		}

		{
			constructorParams := &power0.CreateMinerParams{
				Owner:         m.Worker,
				Worker:        m.Worker,
				Peer:          []byte(m.PeerId),
				SealProofType: spt,
			}

			params := mustEnc(constructorParams)
			rval, err := doExecValue(ctx, vm, power.Address, m.Owner, m.PowerBalance, power.Methods.CreateMiner, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}

			var ma power0.CreateMinerReturn
			if err := ma.UnmarshalCBOR(bytes.NewReader(rval)); err != nil {
				return cid.Undef, xerrors.Errorf("unmarshaling CreateMinerReturn: %w", err)
			}

			expma := MinerAddress(uint64(i))
			if ma.IDAddress != expma {
				return cid.Undef, xerrors.Errorf("miner assigned wrong address: %s != %s", ma.IDAddress, expma)
			}
			minerInfos[i].maddr = ma.IDAddress

			_, err = vm.Flush(ctx)
			if err != nil {
				return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
			}

			mact, err := vm.StateTree().GetActor(minerInfos[i].maddr)
			if err != nil {
				return cid.Undef, xerrors.Errorf("getting newly created miner actor: %w", err)
			}

			mst, err := miner.Load(adt.WrapStore(ctx, cst), mact)
			if err != nil {
				return cid.Undef, xerrors.Errorf("getting newly created miner state: %w", err)
			}

			pps, err := mst.GetProvingPeriodStart()
			if err != nil {
				return cid.Undef, xerrors.Errorf("getting newly created miner proving period start: %w", err)
			}

			minerInfos[i].presealExp = (maxPeriods-1)*miner0.WPoStProvingPeriod + pps - 1
		}

		// Add market funds

		if m.MarketBalance.GreaterThan(big.Zero()) {
			params := mustEnc(&minerInfos[i].maddr)
			_, err := doExecValue(ctx, vm, market.Address, m.Worker, m.MarketBalance, market.Methods.AddBalance, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner (add balance): %w", err)
			}
		}

		// Publish preseal deals

		{
			publish := func(params *market.PublishStorageDealsParams) error {
				fmt.Printf("publishing %d storage deals on miner %s with worker %s\n", len(params.Deals), params.Deals[0].Proposal.Provider, m.Worker)

				ret, err := doExecValue(ctx, vm, market.Address, m.Worker, big.Zero(), builtin0.MethodsMarket.PublishStorageDeals, mustEnc(params))
				if err != nil {
					return xerrors.Errorf("failed to create genesis miner (publish deals): %w", err)
				}
				retval, err := market.DecodePublishStorageDealsReturn(ret, nv)
				if err != nil {
					return xerrors.Errorf("failed to create genesis miner (decoding published deals): %w", err)
				}

				ids, err := retval.DealIDs()
				if err != nil {
					return xerrors.Errorf("failed to create genesis miner (getting published dealIDs): %w", err)
				}

				if len(ids) != len(params.Deals) {
					return xerrors.Errorf("failed to create genesis miner (at least one deal was invalid on publication")
				}

				minerInfos[i].dealIDs = append(minerInfos[i].dealIDs, ids...)
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

				dweight, vdweight, err := dealWeight(ctx, vm, minerInfos[i].maddr, []abi.DealID{minerInfos[i].dealIDs[pi]}, 0, minerInfos[i].presealExp, av)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting deal weight: %w", err)
				}

				sectorWeight := builtin.QAPowerForWeight(m.SectorSize, minerInfos[i].presealExp, dweight, vdweight)

				qaPow = types.BigAdd(qaPow, sectorWeight)
			}
		}

		_, err = vm.Flush(ctx)
		if err != nil {
			return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
		}

		pact, err := vm.StateTree().GetActor(power.Address)
		if err != nil {
			return cid.Undef, xerrors.Errorf("getting power actor: %w", err)
		}

		pst, err := power.Load(adt.WrapStore(ctx, cst), pact)
		if err != nil {
			return cid.Undef, xerrors.Errorf("getting power state: %w", err)
		}

		if err = pst.SetTotalQualityAdjPower(qaPow); err != nil {
			return cid.Undef, xerrors.Errorf("setting TotalQualityAdjPower in power state: %w", err)
		}

		if err = pst.SetTotalRawBytePower(rawPow); err != nil {
			return cid.Undef, xerrors.Errorf("setting TotalRawBytePower in power state: %w", err)
		}

		if err = pst.SetThisEpochQualityAdjPower(qaPow); err != nil {
			return cid.Undef, xerrors.Errorf("setting ThisEpochQualityAdjPower in power state: %w", err)
		}

		if err = pst.SetThisEpochRawBytePower(rawPow); err != nil {
			return cid.Undef, xerrors.Errorf("setting ThisEpochRawBytePower in power state: %w", err)
		}

		pcid, err := cst.Put(ctx, pst.GetState())
		if err != nil {
			return cid.Undef, xerrors.Errorf("putting power state: %w", err)
		}

		pact.Head = pcid

		if err = vm.StateTree().SetActor(power.Address, pact); err != nil {
			return cid.Undef, xerrors.Errorf("setting power state: %w", err)
		}

		rewact, err := SetupRewardActor(ctx, cs.StateBlockstore(), big.Zero(), av)
		if err != nil {
			return cid.Undef, xerrors.Errorf("setup reward actor: %w", err)
		}

		if err = vm.StateTree().SetActor(reward.Address, rewact); err != nil {
			return cid.Undef, xerrors.Errorf("set reward actor: %w", err)
		}
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

				dweight, vdweight, err := dealWeight(ctx, vm, minerInfos[i].maddr, params.DealIDs, 0, minerInfos[i].presealExp, av)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting deal weight: %w", err)
				}

				sectorWeight := builtin.QAPowerForWeight(m.SectorSize, minerInfos[i].presealExp, dweight, vdweight)

				// we've added fake power for this sector above, remove it now

				_, err = vm.Flush(ctx)
				if err != nil {
					return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
				}

				pact, err := vm.StateTree().GetActor(power.Address)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting power actor: %w", err)
				}

				pst, err := power.Load(adt.WrapStore(ctx, cst), pact)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting power state: %w", err)
				}

				pc, err := pst.TotalPower()
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting total power: %w", err)
				}

				if err = pst.SetTotalRawBytePower(types.BigSub(pc.RawBytePower, types.NewInt(uint64(m.SectorSize)))); err != nil {
					return cid.Undef, xerrors.Errorf("setting TotalRawBytePower in power state: %w", err)
				}

				if err = pst.SetTotalQualityAdjPower(types.BigSub(pc.QualityAdjPower, sectorWeight)); err != nil {
					return cid.Undef, xerrors.Errorf("setting TotalQualityAdjPower in power state: %w", err)
				}

				pcid, err := cst.Put(ctx, pst.GetState())
				if err != nil {
					return cid.Undef, xerrors.Errorf("putting power state: %w", err)
				}

				pact.Head = pcid

				if err = vm.StateTree().SetActor(power.Address, pact); err != nil {
					return cid.Undef, xerrors.Errorf("setting power state: %w", err)
				}

				baselinePower, rewardSmoothed, err := currentEpochBlockReward(ctx, vm, minerInfos[i].maddr, av)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting current epoch reward: %w", err)
				}

				tpow, err := currentTotalPower(ctx, vm, minerInfos[i].maddr)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting current total power: %w", err)
				}

				pcd := miner0.PreCommitDepositForPower(&rewardSmoothed, tpow.QualityAdjPowerSmoothed, sectorWeight)

				pledge := miner0.InitialPledgeForPower(
					sectorWeight,
					baselinePower,
					tpow.PledgeCollateral,
					&rewardSmoothed,
					tpow.QualityAdjPowerSmoothed,
					circSupply(ctx, vm, minerInfos[i].maddr),
				)

				pledge = big.Add(pcd, pledge)

				fmt.Println(types.FIL(pledge))
				_, err = doExecValue(ctx, vm, minerInfos[i].maddr, m.Worker, pledge, miner.Methods.PreCommitSector, mustEnc(params))
				if err != nil {
					return cid.Undef, xerrors.Errorf("failed to confirm presealed sectors: %w", err)
				}

				// Commit one-by-one, otherwise pledge math tends to explode
				var paramBytes []byte

				if av >= actors.Version6 {
					// TODO: fixup
					confirmParams := &builtin6.ConfirmSectorProofsParams{
						Sectors: []abi.SectorNumber{preseal.SectorID},
					}

					paramBytes = mustEnc(confirmParams)
				} else {
					confirmParams := &builtin0.ConfirmSectorProofsParams{
						Sectors: []abi.SectorNumber{preseal.SectorID},
					}

					paramBytes = mustEnc(confirmParams)
				}

				_, err = doExecValue(ctx, vm, minerInfos[i].maddr, power.Address, big.Zero(), miner.Methods.ConfirmSectorProofsValid, paramBytes)
				if err != nil {
					return cid.Undef, xerrors.Errorf("failed to confirm presealed sectors: %w", err)
				}

				if av >= actors.Version2 {
					// post v0, we need to explicitly Claim this power since ConfirmSectorProofsValid doesn't do it anymore
					claimParams := &power4.UpdateClaimedPowerParams{
						RawByteDelta:         types.NewInt(uint64(m.SectorSize)),
						QualityAdjustedDelta: sectorWeight,
					}

					_, err = doExecValue(ctx, vm, power.Address, minerInfos[i].maddr, big.Zero(), power.Methods.UpdateClaimedPower, mustEnc(claimParams))
					if err != nil {
						return cid.Undef, xerrors.Errorf("failed to confirm presealed sectors: %w", err)
					}

					_, err = vm.Flush(ctx)
					if err != nil {
						return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
					}

					mact, err := vm.StateTree().GetActor(minerInfos[i].maddr)
					if err != nil {
						return cid.Undef, xerrors.Errorf("getting miner actor: %w", err)
					}

					mst, err := miner.Load(adt.WrapStore(ctx, cst), mact)
					if err != nil {
						return cid.Undef, xerrors.Errorf("getting miner state: %w", err)
					}

					if err = mst.EraseAllUnproven(); err != nil {
						return cid.Undef, xerrors.Errorf("failed to erase unproven sectors: %w", err)
					}

					mcid, err := cst.Put(ctx, mst.GetState())
					if err != nil {
						return cid.Undef, xerrors.Errorf("putting miner state: %w", err)
					}

					mact.Head = mcid

					if err = vm.StateTree().SetActor(minerInfos[i].maddr, mact); err != nil {
						return cid.Undef, xerrors.Errorf("setting miner state: %w", err)
					}
				}
			}
		}
	}

	// Sanity-check total network power
	_, err = vm.Flush(ctx)
	if err != nil {
		return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
	}

	pact, err := vm.StateTree().GetActor(power.Address)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting power actor: %w", err)
	}

	pst, err := power.Load(adt.WrapStore(ctx, cst), pact)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting power state: %w", err)
	}

	pc, err := pst.TotalPower()
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting total power: %w", err)
	}

	if !pc.RawBytePower.Equals(rawPow) {
		return cid.Undef, xerrors.Errorf("TotalRawBytePower (%s) doesn't match previously calculated rawPow (%s)", pc.RawBytePower, rawPow)
	}

	if !pc.QualityAdjPower.Equals(qaPow) {
		return cid.Undef, xerrors.Errorf("QualityAdjPower (%s) doesn't match previously calculated qaPow (%s)", pc.QualityAdjPower, qaPow)
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

func (fr *fakeRand) GetChainRandomnessV2(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch * 1000))).Read(out) //nolint
	return out, nil
}

func (fr *fakeRand) GetChainRandomnessV1(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch * 1000))).Read(out) //nolint
	return out, nil
}

func (fr *fakeRand) GetBeaconRandomnessV3(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch))).Read(out) //nolint
	return out, nil
}

func (fr *fakeRand) GetBeaconRandomnessV2(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch))).Read(out) //nolint
	return out, nil
}

func (fr *fakeRand) GetBeaconRandomnessV1(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch))).Read(out) //nolint
	return out, nil
}

func currentTotalPower(ctx context.Context, vm *vm.VM, maddr address.Address) (*power0.CurrentTotalPowerReturn, error) {
	pwret, err := doExecValue(ctx, vm, power.Address, maddr, big.Zero(), builtin0.MethodsPower.CurrentTotalPower, nil)
	if err != nil {
		return nil, err
	}
	var pwr power0.CurrentTotalPowerReturn
	if err := pwr.UnmarshalCBOR(bytes.NewReader(pwret)); err != nil {
		return nil, err
	}

	return &pwr, nil
}

func dealWeight(ctx context.Context, vm *vm.VM, maddr address.Address, dealIDs []abi.DealID, sectorStart, sectorExpiry abi.ChainEpoch, av actors.Version) (abi.DealWeight, abi.DealWeight, error) {
	// TODO: This hack should move to market actor wrapper
	if av <= actors.Version2 {
		params := &market0.VerifyDealsForActivationParams{
			DealIDs:      dealIDs,
			SectorStart:  sectorStart,
			SectorExpiry: sectorExpiry,
		}

		ret, err := doExecValue(ctx, vm,
			market.Address,
			maddr,
			abi.NewTokenAmount(0),
			builtin0.MethodsMarket.VerifyDealsForActivation,
			mustEnc(params),
		)
		if err != nil {
			return big.Zero(), big.Zero(), err
		}
		var weight, verifiedWeight abi.DealWeight
		if av < actors.Version2 {
			var dealWeights market0.VerifyDealsForActivationReturn
			err = dealWeights.UnmarshalCBOR(bytes.NewReader(ret))
			weight = dealWeights.DealWeight
			verifiedWeight = dealWeights.VerifiedDealWeight
		} else {
			var dealWeights market2.VerifyDealsForActivationReturn
			err = dealWeights.UnmarshalCBOR(bytes.NewReader(ret))
			weight = dealWeights.DealWeight
			verifiedWeight = dealWeights.VerifiedDealWeight
		}
		if err != nil {
			return big.Zero(), big.Zero(), err
		}

		return weight, verifiedWeight, nil
	}
	params := &market4.VerifyDealsForActivationParams{Sectors: []market4.SectorDeals{{
		SectorExpiry: sectorExpiry,
		DealIDs:      dealIDs,
	}}}

	var dealWeights market4.VerifyDealsForActivationReturn
	ret, err := doExecValue(ctx, vm,
		market.Address,
		maddr,
		abi.NewTokenAmount(0),
		market.Methods.VerifyDealsForActivation,
		mustEnc(params),
	)
	if err != nil {
		return big.Zero(), big.Zero(), err
	}
	if err := dealWeights.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
		return big.Zero(), big.Zero(), err
	}

	return dealWeights.Sectors[0].DealWeight, dealWeights.Sectors[0].VerifiedDealWeight, nil
}

func currentEpochBlockReward(ctx context.Context, vm *vm.VM, maddr address.Address, av actors.Version) (abi.StoragePower, builtin.FilterEstimate, error) {
	rwret, err := doExecValue(ctx, vm, reward.Address, maddr, big.Zero(), reward.Methods.ThisEpochReward, nil)
	if err != nil {
		return big.Zero(), builtin.FilterEstimate{}, err
	}

	// TODO: This hack should move to reward actor wrapper
	switch av {
	case actors.Version0:
		var epochReward reward0.ThisEpochRewardReturn

		if err := epochReward.UnmarshalCBOR(bytes.NewReader(rwret)); err != nil {
			return big.Zero(), builtin.FilterEstimate{}, err
		}

		return epochReward.ThisEpochBaselinePower, *epochReward.ThisEpochRewardSmoothed, nil
	case actors.Version2:
		var epochReward reward2.ThisEpochRewardReturn

		if err := epochReward.UnmarshalCBOR(bytes.NewReader(rwret)); err != nil {
			return big.Zero(), builtin.FilterEstimate{}, err
		}

		return epochReward.ThisEpochBaselinePower, builtin.FilterEstimate(epochReward.ThisEpochRewardSmoothed), nil
	}

	var epochReward reward4.ThisEpochRewardReturn

	if err := epochReward.UnmarshalCBOR(bytes.NewReader(rwret)); err != nil {
		return big.Zero(), builtin.FilterEstimate{}, err
	}

	return epochReward.ThisEpochBaselinePower, builtin.FilterEstimate(epochReward.ThisEpochRewardSmoothed), nil
}

func circSupply(ctx context.Context, vmi *vm.VM, maddr address.Address) abi.TokenAmount {
	unsafeVM := &vm.UnsafeVM{VM: vmi}
	rt := unsafeVM.MakeRuntime(ctx, &types.Message{
		GasLimit: 1_000_000_000,
		From:     maddr,
	})

	return rt.TotalFilCircSupply()
}
