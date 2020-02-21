package genesis

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"

	cborutil "github.com/filecoin-project/go-cbor-util"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
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

	vm, err := vm.NewVM(sroot, 0, &fakeRand{}, actors.SystemAddress, cs.Blockstore(), cs.VMSys())
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create NewVM: %w", err)
	}

	err = vm.MutateState(ctx, builtin.StoragePowerActorAddr, func(cst cbor.IpldStore, st *power.State) error {
		st.TotalNetworkPower = networkPower
		return nil
	})

	if len(miners) == 0 {
		return cid.Undef, xerrors.New("no genesis miners")
	}

	for i, m := range miners {
		// Create miner through power actor

		var maddr address.Address
		{
			constructorParams := &power.CreateMinerParams{
				Worker:     m.Worker,
				SectorSize: m.SectorSize,
				Peer:       m.PeerId,
			}

			params := mustEnc(constructorParams)
			rval, err := doExecValue(ctx, vm, actors.StoragePowerAddress, m.Owner, m.PowerBalance, actors.SPAMethods.CreateMiner, params)
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
			_, err := doExecValue(ctx, vm, actors.StorageMarketAddress, m.Worker, m.MarketBalance, builtin.MethodsMarket.AddBalance, params)
			if err != nil {
				return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
			}
		}
		{
			params := mustEnc(&m.Worker)
			_, err := doExecValue(ctx, vm, actors.StorageMarketAddress, m.Worker, big.Zero(), builtin.MethodsMarket.AddBalance, params)
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
					ClientSignature: crypto.Signature{Type:crypto.SigTypeBLS}, // TODO: do we want to sign these? Or do we want to fake signatures for genesis setup?
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
			err = vm.MutateState(ctx, maddr, func(cst cbor.IpldStore, st *miner.State) error {
				// TODO: Randomize so all genesis miners don't fall on the same epoch
				st.PoStState.ProvingPeriodStart = miner.ProvingPeriod
				return nil
			})

			payload, err := cborutil.Dump(&miner.CronEventPayload{
				EventType: miner.CronEventWindowedPoStExpiration,
			})
			if err != nil {
				return cid.Undef, err
			}
			params := &power.EnrollCronEventParams{
				EventEpoch: miner.ProvingPeriod,
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
			pledge := big.Zero()
			{
				err = vm.MutateState(ctx, builtin.StoragePowerActorAddr, func(cst cbor.IpldStore, st *power.State) error {
					weight := &power.SectorStorageWeightDesc{
						SectorSize: m.SectorSize,
						Duration:   preseal.Deal.Duration(),
						DealWeight: dealWeight,
					}

					spower := power.ConsensusPowerForWeight(weight)
					pledge = power.PledgeForWeight(weight, big.Sub(st.TotalNetworkPower, spower))
					err := st.AddToClaim(&state.AdtStore{cst}, maddr, spower, pledge)
					if err != nil {
						return xerrors.Errorf("add to claim: %w", err)
					}
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
						SectorNumber: preseal.SectorID,
						SealedCID:    commcid.ReplicaCommitmentV1ToCID(preseal.CommR[:]),
						SealRandEpoch:    0,
						DealIDs:      []abi.DealID{dealIDs[pi]},
						Expiration:   preseal.Deal.EndEpoch,
					},
					ActivationEpoch:   0,
					DealWeight:        dealWeight,
					PledgeRequirement: pledge,
				}

				err = vm.MutateState(ctx, maddr, func(cst cbor.IpldStore, st *miner.State) error {
					store := &state.AdtStore{cst}

					if err = st.PutSector(store, newSectorInfo); err != nil {
						return xerrors.Errorf("failed to prove commit: %v", err)
					}

					st.ProvingSet = st.Sectors
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
					EventType: miner.CronEventSectorExpiry,
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

	c, err := vm.Flush(ctx)
	return c, err
}

// TODO: copied from actors test harness, deduplicate or remove from here
type fakeRand struct{}

func (fr *fakeRand) GetRandomness(ctx context.Context, h int64) ([]byte, error) {
	out := make([]byte, 32)
	rand.New(rand.NewSource(h)).Read(out)
	return out, nil
}
