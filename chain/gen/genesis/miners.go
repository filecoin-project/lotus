package genesis

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/crypto"

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

func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sroot cid.Cid, miners []genesis.Miner) (cid.Cid, error) {
	networkPower := big.Zero()
	for _, m := range miners {
		networkPower = big.Add(networkPower, big.NewInt(int64(m.SectorSize)*int64(len(m.Sectors))))
	}

	vm, err := vm.NewVM(sroot, 0, &fakeRand{}, builtin.SystemActorAddr, cs.Blockstore(), cs.VMSys())
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
			constructorParams := &power.CreateMinerParams{
				Owner:      m.Worker,
				Worker:     m.Worker,
				SectorSize: m.SectorSize,
				Peer:       m.PeerId,
			}

			params := mustEnc(constructorParams)
			rval, err := doExecValue(ctx, vm, builtin.StoragePowerActorAddr, m.Owner, m.PowerBalance, builtin.MethodsPower.CreateMiner, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}

			var ma power.CreateMinerReturn
			if err := ma.UnmarshalCBOR(bytes.NewReader(rval)); err != nil {
				return cid.Undef, err
			}

			expma := MinerAddress(uint64(i))
			if ma.IDAddress != expma {
				return cid.Undef, xerrors.Errorf("miner assigned wrong address: %s != %s", ma.IDAddress, expma)
			}
			maddr = ma.IDAddress
		}

		// Add market funds

		{
			params := mustEnc(&maddr)
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

		var dealIDs []abi.DealID
		{
			params := &market.PublishStorageDealsParams{}
			for _, preseal := range m.Sectors {

				params.Deals = append(params.Deals, market.ClientDealProposal{
					Proposal:        preseal.Deal,
					ClientSignature: crypto.Signature{Type: crypto.SigTypeBLS}, // TODO: do we want to sign these? Or do we want to fake signatures for genesis setup?
				})
				fmt.Printf("calling publish storage deals on miner %s with worker %s\n", preseal.Deal.Provider, m.Worker)
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

		// setup windowed post
		{
			provingPeriodBoundary := abi.ChainEpoch(0)

			err = vm.MutateState(ctx, maddr, func(cst cbor.IpldStore, st *miner.State) error {
				panic("from some randomness")
				st.ProvingPeriodBoundary = 0
				return nil
			})

			panic("todo, probably more cron stuff")
			payload, err := cborutil.Dump(&miner.CronEventPayload{
				EventType: miner.CronEventProvingPeriod,
			})
			if err != nil {
				return cid.Undef, err
			}
			params := &power.EnrollCronEventParams{
				EventEpoch: miner.WPoStProvingPeriod + provingPeriodBoundary, // TODO: correct ???
				Payload:    payload,
			}

			_, err = doExecValue(ctx, vm, builtin.StoragePowerActorAddr, maddr, big.Zero(), builtin.MethodsPower.EnrollCronEvent, mustEnc(params))
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to verify preseal deals miner: %w", err)
			}
		}

		// Commit sectors
		for pi, preseal := range m.Sectors {
			// TODO: Maybe check seal (Can just be snark inputs, doesn't go into the genesis file)

			// check deals, get dealWeight
			dealWeight := big.Zero()
			{
				params := &market.VerifyDealsOnSectorProveCommitParams{
					DealIDs:      []abi.DealID{dealIDs[pi]},
					SectorSize:   m.SectorSize,
					SectorExpiry: preseal.Deal.EndEpoch,
				}

				ret, err := doExecValue(ctx, vm, builtin.StorageMarketActorAddr, maddr, big.Zero(), builtin.MethodsMarket.VerifyDealsOnSectorProveCommit, mustEnc(params))
				if err != nil {
					return cid.Undef, xerrors.Errorf("failed to verify preseal deals miner: %w", err)
				}
				if err := dealWeight.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
					return cid.Undef, err
				}
			}

			// update power claims
			{
				err = vm.MutateState(ctx, builtin.StoragePowerActorAddr, func(cst cbor.IpldStore, st *power.State) error {
					weight := &power.SectorStorageWeightDesc{
						SectorSize: m.SectorSize,
						Duration:   preseal.Deal.Duration(),
						DealWeight: dealWeight,
					}

					qapower := power.QAPowerForWeight(weight)

					err := st.AddToClaim(&state.AdtStore{cst}, maddr, types.NewInt(uint64(weight.SectorSize)), qapower)
					if err != nil {
						return xerrors.Errorf("add to claim: %w", err)
					}
					fmt.Println("Added weight to claim: ", st.TotalRawBytePower, st.TotalQualityAdjPower)
					return nil
				})
				if err != nil {
					return cid.Undef, xerrors.Errorf("register power claim in power actor: %w", err)
				}
			}

			// Put sectors to miner sector sets
			{
				newSectorInfo := &miner.SectorOnChainInfo{
					Info: miner.SectorPreCommitInfo{
						RegisteredProof: preseal.ProofType,
						SectorNumber:    preseal.SectorID,
						SealedCID:       preseal.CommR,
						SealRandEpoch:   0,
						DealIDs:         []abi.DealID{dealIDs[pi]},
						Expiration:      preseal.Deal.EndEpoch,
					},
					ActivationEpoch: 0,
					DealWeight:      dealWeight,
				}

				err = vm.MutateState(ctx, maddr, func(cst cbor.IpldStore, st *miner.State) error {
					store := &state.AdtStore{cst}

					if err = st.PutSector(store, newSectorInfo); err != nil {
						return xerrors.Errorf("failed to prove commit: %v", err)
					}

					panic("assign deadlines")
					return nil
				})
				if err != nil {
					return cid.Cid{}, xerrors.Errorf("put to sset: %w", err)
				}
			}

			{
				sectorBf := abi.NewBitField()
				sectorBf.Set(uint64(preseal.SectorID))

				payload, err := cborutil.Dump(&miner.CronEventPayload{
					EventType: miner.CronEventPreCommitExpiry, // TODO: Review: Is this correct?
					Sectors:   &sectorBf,
				})
				if err != nil {
					return cid.Undef, err
				}
				params := &power.EnrollCronEventParams{
					EventEpoch: preseal.Deal.EndEpoch,
					Payload:    payload,
				}

				_, err = doExecValue(ctx, vm, builtin.StoragePowerActorAddr, maddr, big.Zero(), builtin.MethodsPower.EnrollCronEvent, mustEnc(params))
				if err != nil {
					return cid.Undef, xerrors.Errorf("failed to verify preseal deals miner: %w", err)
				}
			}
		}

	}

	// TODO: to avoid division by zero, we set the initial power actor power to 1, this adjusts that back down so the accounting is accurate.
	err = vm.MutateState(ctx, builtin.StoragePowerActorAddr, func(cst cbor.IpldStore, st *power.State) error {
		st.TotalQualityAdjPower = big.Sub(st.TotalQualityAdjPower, big.NewInt(1))
		return nil
	})

	c, err := vm.Flush(ctx)
	return c, err
}

// TODO: copied from actors test harness, deduplicate or remove from here
type fakeRand struct{}

func (fr *fakeRand) GetRandomness(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, _ = rand.New(rand.NewSource(int64(randEpoch))).Read(out)
	return out, nil
}
