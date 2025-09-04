package genesis

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	power11 "github.com/filecoin-project/go-state-types/builtin/v11/power"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	power15 "github.com/filecoin-project/go-state-types/builtin/v15/power"
	smoothing15 "github.com/filecoin-project/go-state-types/builtin/v15/util/smoothing"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	power0 "github.com/filecoin-project/specs-actors/actors/builtin/power"
	reward0 "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	power2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	reward2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/reward"
	power4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/power"
	reward4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/reward"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	runtime7 "github.com/filecoin-project/specs-actors/v7/actors/runtime"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/actors/builtin/system"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus"
	lrand "github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/sigs"
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

func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sys vm.SyscallBuilder, sroot cid.Cid, miners []genesis.Miner, nv network.Version, synthetic bool) (cid.Cid, error) {

	cst := cbor.NewCborStore(cs.StateBlockstore())
	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get network version: %w", err)
	}

	csc := func(context.Context, abi.ChainEpoch, *state.StateTree) (abi.TokenAmount, error) {
		return big.Zero(), nil
	}

	newVM := func(base cid.Cid) (vm.Interface, error) {
		vmopt := &vm.VMOpts{
			StateBase:      base,
			Epoch:          0,
			Rand:           &fakeRand{},
			Bstore:         cs.StateBlockstore(),
			Actors:         consensus.NewActorRegistry(),
			Syscalls:       mkFakedSigSyscalls(sys),
			CircSupplyCalc: csc,
			NetworkVersion: nv,
			BaseFee:        big.Zero(),
		}

		return vm.NewVM(ctx, vmopt)
	}

	genesisVm, err := newVM(sroot)
	if err != nil {
		return cid.Undef, fmt.Errorf("creating vm: %w", err)
	}

	if len(miners) == 0 {
		return cid.Undef, xerrors.New("no genesis miners")
	}

	minerInfos := make([]struct {
		maddr address.Address

		presealExp abi.ChainEpoch

		dealIDs      []abi.DealID
		sectorWeight []abi.StoragePower
	}, len(miners))

	maxLifetime, err := policy.GetMaxSectorExpirationExtension(nv)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get max extension: %w", err)
	}
	maxPeriods := maxLifetime / minertypes.WPoStProvingPeriod
	rawPow, qaPow := big.NewInt(0), big.NewInt(0)
	for i, m := range miners {
		// Create miner through power actor
		i := i
		m := m

		variant := miner.SealProofVariant_Standard
		if synthetic {
			variant = miner.SealProofVariant_Synthetic
		}
		spt, err := miner.SealProofTypeFromSectorSize(m.SectorSize, nv, variant)
		if err != nil {
			return cid.Undef, err
		}

		{
			var params []byte
			if nv <= network.Version10 {
				constructorParams := &power2.CreateMinerParams{
					Owner:         m.Worker,
					Worker:        m.Worker,
					Peer:          []byte(m.PeerId),
					SealProofType: spt,
				}

				params = mustEnc(constructorParams)
			} else {
				ppt, err := spt.RegisteredWindowPoStProofByNetworkVersion(nv)
				if err != nil {
					return cid.Undef, xerrors.Errorf("failed to convert spt to wpt: %w", err)
				}

				constructorParams := &power11.CreateMinerParams{
					Owner:               m.Worker,
					Worker:              m.Worker,
					Peer:                []byte(m.PeerId),
					WindowPoStProofType: ppt,
				}

				params = mustEnc(constructorParams)
			}

			deposit, err := CalculateMinerCreationDepositForGenesis(ctx, genesisVm, adt.WrapStore(ctx, cst))
			if err != nil {
				return cid.Undef, xerrors.Errorf("calculating miner creation deposit: %w", err)
			}

			rval, err := doExecValue(ctx, genesisVm, power.Address, m.Owner, deposit, power.Methods.CreateMiner, params)
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

			nh, err := genesisVm.Flush(ctx)
			if err != nil {
				return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
			}

			nst, err := state.LoadStateTree(cst, nh)
			if err != nil {
				return cid.Undef, xerrors.Errorf("loading new state tree: %w", err)
			}

			mact, err := nst.GetActor(minerInfos[i].maddr)
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
			_, err := doExecValue(ctx, genesisVm, market.Address, m.Worker, m.MarketBalance, market.Methods.AddBalance, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner (add balance): %w", err)
			}
		}

		// Publish preseal deals, and calculate the QAPower

		{
			publish := func(params *markettypes.PublishStorageDealsParams) error {
				fmt.Printf("publishing %d storage deals on miner %s with worker %s\n", len(params.Deals), params.Deals[0].Proposal.Provider, m.Worker)

				ret, err := doExecValue(ctx, genesisVm, market.Address, m.Worker, big.Zero(), builtin0.MethodsMarket.PublishStorageDeals, mustEnc(params))
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

			params := &markettypes.PublishStorageDealsParams{}
			for _, presealTmp := range m.Sectors {
				preseal := presealTmp
				preseal.Deal.VerifiedDeal = true
				preseal.Deal.EndEpoch = minerInfos[i].presealExp
				p := markettypes.ClientDealProposal{
					Proposal:        preseal.Deal,
					ClientSignature: crypto.Signature{Type: crypto.SigTypeBLS},
				}

				if av >= actorstypes.Version8 {
					buf, err := cborutil.Dump(&preseal.Deal)
					if err != nil {
						return cid.Undef, fmt.Errorf("failed to marshal proposal: %w", err)
					}

					sig, err := sigs.Sign(key.ActSigType(preseal.DealClientKey.Type), preseal.DealClientKey.PrivateKey, buf)
					if err != nil {
						return cid.Undef, fmt.Errorf("failed to sign proposal: %w", err)
					}

					p.ClientSignature = *sig
				}

				params.Deals = append(params.Deals, p)

				if len(params.Deals) == cbg.MaxLength {
					if err := publish(params); err != nil {
						return cid.Undef, err
					}

					params = &markettypes.PublishStorageDealsParams{}
				}

				rawPow = big.Add(rawPow, big.NewInt(int64(m.SectorSize)))
				sectorWeight := builtin.QAPowerForWeight(m.SectorSize, minerInfos[i].presealExp, markettypes.DealWeight(&preseal.Deal))
				minerInfos[i].sectorWeight = append(minerInfos[i].sectorWeight, sectorWeight)
				qaPow = big.Add(qaPow, sectorWeight)
			}

			if len(params.Deals) > 0 {
				if err := publish(params); err != nil {
					return cid.Undef, err
				}
			}
		}
	}

	{
		nh, err := genesisVm.Flush(ctx)
		if err != nil {
			return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
		}

		if err != nil {
			return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
		}

		nst, err := state.LoadStateTree(cst, nh)
		if err != nil {
			return cid.Undef, xerrors.Errorf("loading new state tree: %w", err)
		}

		pact, err := nst.GetActor(power.Address)
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

		if err = nst.SetActor(power.Address, pact); err != nil {
			return cid.Undef, xerrors.Errorf("setting power state: %w", err)
		}

		rewact, err := SetupRewardActor(ctx, cs.StateBlockstore(), big.Zero(), av)
		if err != nil {
			return cid.Undef, xerrors.Errorf("setup reward actor: %w", err)
		}

		if err = nst.SetActor(reward.Address, rewact); err != nil {
			return cid.Undef, xerrors.Errorf("set reward actor: %w", err)
		}

		nh, err = nst.Flush(ctx)
		if err != nil {
			return cid.Undef, xerrors.Errorf("flushing state tree: %w", err)
		}

		genesisVm, err = newVM(nh)
		if err != nil {
			return cid.Undef, fmt.Errorf("creating new vm: %w", err)
		}
	}

	for i, m := range miners {
		// Commit sectors
		{
			for pi, preseal := range m.Sectors {
				var paramEnc []byte
				var preCommitMethodNum abi.MethodNum
				if nv >= network.Version22 {
					paramEnc = mustEnc(&miner.PreCommitSectorBatchParams2{
						Sectors: []miner.SectorPreCommitInfo{
							{
								SealProof:     preseal.ProofType,
								SectorNumber:  preseal.SectorID,
								SealedCID:     preseal.CommR,
								SealRandEpoch: -1,
								DealIDs:       []abi.DealID{minerInfos[i].dealIDs[pi]},
								Expiration:    minerInfos[i].presealExp, // TODO: Allow setting externally!
								UnsealedCid:   &preseal.CommD,
							},
						},
					})
					preCommitMethodNum = builtintypes.MethodsMiner.PreCommitSectorBatch2
				} else {
					paramEnc = mustEnc(&minertypes.SectorPreCommitInfo{
						SealProof:     preseal.ProofType,
						SectorNumber:  preseal.SectorID,
						SealedCID:     preseal.CommR,
						SealRandEpoch: -1,
						DealIDs:       []abi.DealID{minerInfos[i].dealIDs[pi]},
						Expiration:    minerInfos[i].presealExp, // TODO: Allow setting externally!
					})
					preCommitMethodNum = builtintypes.MethodsMiner.PreCommitSector
				}

				sectorWeight := minerInfos[i].sectorWeight[pi]

				// we've added fake power for this sector above, remove it now

				nh, err := genesisVm.Flush(ctx)
				if err != nil {
					return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
				}

				nst, err := state.LoadStateTree(cst, nh)
				if err != nil {
					return cid.Undef, xerrors.Errorf("loading new state tree: %w", err)
				}

				pact, err := nst.GetActor(power.Address)
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

				if err = nst.SetActor(power.Address, pact); err != nil {
					return cid.Undef, xerrors.Errorf("setting power state: %w", err)
				}

				nh, err = nst.Flush(ctx)
				if err != nil {
					return cid.Undef, xerrors.Errorf("flushing state tree: %w", err)
				}

				genesisVm, err = newVM(nh)
				if err != nil {
					return cid.Undef, fmt.Errorf("creating new vm: %w", err)
				}

				baselinePower, rewardSmoothed, err := currentEpochBlockReward(ctx, genesisVm, minerInfos[i].maddr, av)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting current epoch reward: %w", err)
				}

				qaps, err := currentQualityAdjPowerSmoothed(ctx, nv, genesisVm, minerInfos[i].maddr)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting current total power: %w", err)
				}

				pcd := miner15.PreCommitDepositForPower(smoothing15.FilterEstimate(rewardSmoothed), qaps, miner15.QAPowerMax(m.SectorSize))

				pledge := miner15.InitialPledgeForPower(
					sectorWeight,
					baselinePower,
					smoothing15.FilterEstimate(rewardSmoothed),
					qaps,
					big.Zero(),
					// epochsSinceRampStart and rampDurationEpochs: InternalSectorSetupForPreseal uses 0,0 for
					// these parameters, regardless of what the power actor state says.
					0, 0,
				)

				pledge = big.Add(pcd, pledge)

				_, err = doExecValue(ctx, genesisVm, minerInfos[i].maddr, m.Worker, pledge, preCommitMethodNum, paramEnc)
				if err != nil {
					return cid.Undef, xerrors.Errorf("failed to confirm presealed sectors: %w", err)
				}

				// Commit one-by-one, otherwise pledge math tends to explode
				var paramBytes []byte

				if av >= actorstypes.Version14 {
					confirmParams := &miner14.InternalSectorSetupForPresealParams{
						Sectors: []abi.SectorNumber{preseal.SectorID},
					}
					paramBytes = mustEnc(confirmParams)
				} else if av >= actorstypes.Version6 {
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

				var csErr error
				if nv >= network.Version23 {
					_, csErr = doExecValue(ctx, genesisVm, minerInfos[i].maddr, system.Address, big.Zero(), builtintypes.MethodsMiner.InternalSectorSetupForPreseal,
						paramBytes)
				} else {
					_, csErr = doExecValue(ctx, genesisVm, minerInfos[i].maddr, power.Address, big.Zero(), builtintypes.MethodsMiner.InternalSectorSetupForPreseal,
						paramBytes)
				}

				if csErr != nil {
					return cid.Undef, xerrors.Errorf("failed to confirm presealed sectors: %w", csErr)
				}

				if av >= actorstypes.Version2 {
					// post v0, we need to explicitly Claim this power since ConfirmSectorProofsValid doesn't do it anymore
					claimParams := &power4.UpdateClaimedPowerParams{
						RawByteDelta:         types.NewInt(uint64(m.SectorSize)),
						QualityAdjustedDelta: sectorWeight,
					}

					_, err = doExecValue(ctx, genesisVm, power.Address, minerInfos[i].maddr, big.Zero(), power.Methods.UpdateClaimedPower, mustEnc(claimParams))
					if err != nil {
						return cid.Undef, xerrors.Errorf("failed to confirm presealed sectors: %w", err)
					}

					nh, err := genesisVm.Flush(ctx)
					if err != nil {
						return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
					}

					nst, err := state.LoadStateTree(cst, nh)
					if err != nil {
						return cid.Undef, xerrors.Errorf("loading new state tree: %w", err)
					}

					mact, err := nst.GetActor(minerInfos[i].maddr)
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

					if err = nst.SetActor(minerInfos[i].maddr, mact); err != nil {
						return cid.Undef, xerrors.Errorf("setting miner state: %w", err)
					}

					nh, err = nst.Flush(ctx)
					if err != nil {
						return cid.Undef, xerrors.Errorf("flushing state tree: %w", err)
					}

					genesisVm, err = newVM(nh)
					if err != nil {
						return cid.Undef, fmt.Errorf("creating new vm: %w", err)
					}
				}
			}
		}
	}

	// Sanity-check total network power
	nh, err := genesisVm.Flush(ctx)
	if err != nil {
		return cid.Undef, fmt.Errorf("flushing vm: %w", err)
	}

	nst, err := state.LoadStateTree(cst, nh)
	if err != nil {
		return cid.Undef, xerrors.Errorf("loading new state tree: %w", err)
	}

	pact, err := nst.GetActor(power.Address)
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

	c, err := genesisVm.Flush(ctx)
	if err != nil {
		return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
	}

	return c, nil
}

var _ lrand.Rand = new(fakeRand)

// TODO: copied from actors test harness, deduplicate or remove from here
type fakeRand struct{}

func (fr *fakeRand) GetChainRandomness(ctx context.Context, randEpoch abi.ChainEpoch) ([32]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch * 1000))).Read(out) //nolint
	return *(*[32]byte)(out), nil
}

func (fr *fakeRand) GetBeaconEntry(ctx context.Context, randEpoch abi.ChainEpoch) (*types.BeaconEntry, error) {
	r, _ := fr.GetChainRandomness(ctx, randEpoch)
	return &types.BeaconEntry{Round: 10, Data: r[:]}, nil
}

func (fr *fakeRand) GetBeaconRandomness(ctx context.Context, randEpoch abi.ChainEpoch) ([32]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch))).Read(out) //nolint
	return *(*[32]byte)(out), nil
}

func currentQualityAdjPowerSmoothed(ctx context.Context, nv network.Version, vm vm.Interface, maddr address.Address) (smoothing15.FilterEstimate, error) {
	pwret, err := doExecValue(ctx, vm, power.Address, maddr, big.Zero(), builtin0.MethodsPower.CurrentTotalPower, nil)
	if err != nil {
		return smoothing15.FilterEstimate{}, err
	}
	if nv >= network.Version24 {
		var pwr power15.CurrentTotalPowerReturn
		if err := pwr.UnmarshalCBOR(bytes.NewReader(pwret)); err != nil {
			return smoothing15.FilterEstimate{}, err
		}
		return pwr.QualityAdjPowerSmoothed, nil
	}
	var pwr power0.CurrentTotalPowerReturn
	if err := pwr.UnmarshalCBOR(bytes.NewReader(pwret)); err != nil {
		return smoothing15.FilterEstimate{}, err
	}
	return smoothing15.FilterEstimate(*pwr.QualityAdjPowerSmoothed), nil
}

func currentEpochBlockReward(ctx context.Context, vm vm.Interface, maddr address.Address, av actorstypes.Version) (abi.StoragePower, builtin.FilterEstimate, error) {
	rwret, err := doExecValue(ctx, vm, reward.Address, maddr, big.Zero(), reward.Methods.ThisEpochReward, nil)
	if err != nil {
		return big.Zero(), builtin.FilterEstimate{}, err
	}

	// TODO: This hack should move to reward actor wrapper
	switch av {
	case actorstypes.Version0:
		var epochReward reward0.ThisEpochRewardReturn

		if err := epochReward.UnmarshalCBOR(bytes.NewReader(rwret)); err != nil {
			return big.Zero(), builtin.FilterEstimate{}, err
		}

		return epochReward.ThisEpochBaselinePower, builtin.FilterEstimate(*epochReward.ThisEpochRewardSmoothed), nil
	case actorstypes.Version2:
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
