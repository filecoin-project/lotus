package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"
	v10 "github.com/filecoin-project/go-state-types/builtin/v10"
	market8 "github.com/filecoin-project/go-state-types/builtin/v8/market"
	adt8 "github.com/filecoin-project/go-state-types/builtin/v8/util/adt"
	v9 "github.com/filecoin-project/go-state-types/builtin/v9"
	market9 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	adt9 "github.com/filecoin-project/go-state-types/builtin/v9/util/adt"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/manifest"
	mutil "github.com/filecoin-project/go-state-types/migration"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/v7/actors/migration/nv15"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	lbuiltin "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
)

var migrationsCmd = &cli.Command{
	Name:        "migrate-state",
	Description: "Run a network upgrade migration",
	ArgsUsage:   "[new network version, block to look back from]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.BoolFlag{
			Name: "skip-pre-migration",
		},
		&cli.BoolFlag{
			Name: "check-invariants",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		nv, err := strconv.ParseUint(cctx.Args().Get(0), 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse network version: %w", err)
		}

		upgradeActorsFunc, preUpgradeActorsFunc, checkInvariantsFunc, err := getMigrationFuncsForNetwork(network.Version(nv))
		if err != nil {
			return err
		}

		blkCid, err := cid.Decode(cctx.Args().Get(1))
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		bs, err := lkrepo.Blockstore(ctx, repo.UniversalBlockstore)
		if err != nil {
			return fmt.Errorf("failed to open blockstore: %w", err)
		}

		defer func() {
			if c, ok := bs.(io.Closer); ok {
				if err := c.Close(); err != nil {
					log.Warnf("failed to close blockstore: %s", err)
				}
			}
		}()

		mds, err := lkrepo.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		// Note: we use a map datastore for the metadata to avoid writing / using cached migration results in the metadata store
		sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(ffiwrapper.ProofVerifier), filcns.DefaultUpgradeSchedule(), nil, datastore.NewMapDatastore())
		if err != nil {
			return err
		}

		blk, err := cs.GetBlock(ctx, blkCid)
		if err != nil {
			return err
		}

		migrationTs, err := cs.LoadTipSet(ctx, types.NewTipSetKey(blk.Parents...))
		if err != nil {
			return err
		}

		startTime := time.Now()

		newCid2, err := upgradeActorsFunc(ctx, sm, nv15.NewMemMigrationCache(), nil, blk.ParentStateRoot, blk.Height-1, migrationTs)
		if err != nil {
			return err
		}

		uncachedMigrationTime := time.Since(startTime)

		fmt.Println("migration height ", blk.Height-1)
		fmt.Println("old cid ", blk.ParentStateRoot)
		fmt.Println("new cid ", newCid2)
		fmt.Println("completed round actual (without cache), took ", uncachedMigrationTime)

		if !cctx.IsSet("skip-pre-migration") {
			cache := mutil.NewMemMigrationCache()

			ts1, err := cs.GetTipsetByHeight(ctx, blk.Height-60, migrationTs, false)
			if err != nil {
				return err
			}

			startTime = time.Now()

			err = preUpgradeActorsFunc(ctx, sm, cache, ts1.ParentState(), ts1.Height()-1, ts1)
			if err != nil {
				return err
			}

			preMigrationTime := time.Since(startTime)

			startTime = time.Now()

			newCid1, err := upgradeActorsFunc(ctx, sm, cache, nil, blk.ParentStateRoot, blk.Height-1, migrationTs)
			if err != nil {
				return err
			}

			cachedMigrationTime := time.Since(startTime)

			if newCid1 != newCid2 {
				return xerrors.Errorf("got different results with and without the cache: %s, %s", newCid1,
					newCid2)
			}
			fmt.Println("completed premigration, took ", preMigrationTime)
			fmt.Println("completed round actual (with cache), took ", cachedMigrationTime)
		}

		if cctx.Bool("check-invariants") {
			if checkInvariantsFunc == nil {
				return xerrors.Errorf("check invariants not implemented for nv%d", nv)
			}
			err = checkInvariantsFunc(ctx, blk.ParentStateRoot, newCid2, bs, blk.Height-1)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

func getMigrationFuncsForNetwork(nv network.Version) (UpgradeActorsFunc, PreUpgradeActorsFunc, CheckInvariantsFunc, error) {
	switch nv {
	case network.Version17:
		return filcns.UpgradeActorsV9, filcns.PreUpgradeActorsV9, checkNv17Invariants, nil
	case network.Version18:
		return filcns.UpgradeActorsV10, filcns.PreUpgradeActorsV10, checkNv18Invariants, nil
	default:
		return nil, nil, nil, xerrors.Errorf("migration not implemented for nv%d", nv)
	}
}

type UpgradeActorsFunc = func(context.Context, *stmgr.StateManager, stmgr.MigrationCache, stmgr.ExecMonitor, cid.Cid, abi.ChainEpoch, *types.TipSet) (cid.Cid, error)
type PreUpgradeActorsFunc = func(context.Context, *stmgr.StateManager, stmgr.MigrationCache, cid.Cid, abi.ChainEpoch, *types.TipSet) error
type CheckInvariantsFunc = func(context.Context, cid.Cid, cid.Cid, blockstore.Blockstore, abi.ChainEpoch) error

func checkNv18Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {
	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version10)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v10.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

/// NV17 and earlier stuff

func checkNv17Invariants(ctx context.Context, v8StateRootCid cid.Cid, v9StateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {
	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	stateTreeV8, err := state.LoadStateTree(actorStore, v8StateRootCid)
	if err != nil {
		return err
	}

	stateTreeV9, err := state.LoadStateTree(actorStore, v9StateRootCid)
	if err != nil {
		return err
	}

	err = checkDatacaps(stateTreeV8, stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	err = checkPendingVerifiedDeals(stateTreeV8, stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	err = checkAllMinersUnsealedCID(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	// Load the state root.
	var v9stateRoot types.StateRoot
	if err := actorStore.Get(ctx, v9StateRootCid, &v9stateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version9)
	if err != nil {
		return err
	}
	v9actorTree, err := builtin.LoadTree(actorStore, v9stateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v9.CheckStateInvariants(v9actorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

func checkDatacaps(stateTreeV8 *state.StateTree, stateTreeV9 *state.StateTree, actorStore adt.Store) error {
	verifregDatacaps, err := getVerifreg8Datacaps(stateTreeV8, actorStore)
	if err != nil {
		return err
	}

	newDatacaps, err := getDatacap9Datacaps(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	// Should have all the v8 datacaps, plus the verifreg actor itself
	if len(verifregDatacaps)+1 != len(newDatacaps) {
		return xerrors.Errorf("size of datacap maps do not match. verifreg: %d, datacap: %d", len(verifregDatacaps), len(newDatacaps))
	}

	for addr, oldDcap := range verifregDatacaps {
		dcap, ok := newDatacaps[addr]
		if !ok {
			return xerrors.Errorf("datacap for address: %s not found in datacap state", addr)
		}
		if !dcap.Equals(oldDcap) {
			return xerrors.Errorf("datacap for address: %s do not match. verifreg: %d, datacap: %d", addr, oldDcap, dcap)
		}
	}

	return nil
}

func getVerifreg8Datacaps(stateTreeV8 *state.StateTree, actorStore adt.Store) (map[address.Address]abi.StoragePower, error) {
	verifregStateV8, err := getVerifregActorV8(stateTreeV8, actorStore)
	if err != nil {
		return nil, xerrors.Errorf("failed to get verifreg actor state: %w", err)
	}

	var verifregDatacaps = make(map[address.Address]abi.StoragePower)
	err = verifregStateV8.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
		verifregDatacaps[addr] = dcap
		return nil
	})
	if err != nil {
		return nil, err
	}

	return verifregDatacaps, nil
}

func getDatacap9Datacaps(stateTreeV9 *state.StateTree, actorStore adt.Store) (map[address.Address]abi.StoragePower, error) {
	datacapStateV9, err := getDatacapActorV9(stateTreeV9, actorStore)
	if err != nil {
		return nil, xerrors.Errorf("failed to get datacap actor state: %w", err)
	}

	var datacaps = make(map[address.Address]abi.StoragePower)
	err = datacapStateV9.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
		datacaps[addr] = dcap
		return nil
	})
	if err != nil {
		return nil, err
	}

	return datacaps, nil
}

func checkPendingVerifiedDeals(stateTreeV8 *state.StateTree, stateTreeV9 *state.StateTree, actorStore adt.Store) error {
	marketActorV9, err := getMarketActorV9(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	verifregActorV9, err := getVerifregActorV9(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	verifregStateV9, err := getVerifregStateV9(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	marketStateV8, err := getMarketStateV8(stateTreeV8, actorStore)
	if err != nil {
		return err
	}

	marketStateV9, err := getMarketStateV9(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	pendingProposalsV8, err := adt8.AsSet(actorStore, marketStateV8.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return xerrors.Errorf("failed to load pending proposals: %w", err)
	}

	dealProposalsV8, err := market8.AsDealProposalArray(actorStore, marketStateV8.Proposals)
	if err != nil {
		return xerrors.Errorf("failed to get proposals: %w", err)
	}

	// We only want those pending deals that haven't been activated -- an activated deal has an entry in dealStates8
	dealStates8, err := adt9.AsArray(actorStore, marketStateV8.States, market8.StatesAmtBitwidth)
	if err != nil {
		return xerrors.Errorf("failed to load v8 states array: %w", err)
	}

	var numPendingVerifiedDeals = 0
	var proposal market8.DealProposal
	err = dealProposalsV8.ForEach(&proposal, func(dealID int64) error {
		// If not verified, do nothing
		if !proposal.VerifiedDeal {
			return nil
		}

		pcid, err := proposal.Cid()
		if err != nil {
			return err
		}

		isPending, err := pendingProposalsV8.Has(abi.CidKey(pcid))
		if err != nil {
			return xerrors.Errorf("failed to check pending: %w", err)
		}

		// Nothing to do for not-pending deals
		if !isPending {
			return nil
		}

		var _dealState8 market8.DealState
		found, err := dealStates8.Get(uint64(dealID), &_dealState8)
		if err != nil {
			return xerrors.Errorf("failed to lookup deal state: %w", err)
		}

		// the deal has an entry in deal states, which means it's already been allocated, nothing to do
		if found {
			return nil
		}

		numPendingVerifiedDeals++
		// Checks if allocation ID is in market map
		allocationId, err := marketActorV9.GetAllocationIdForPendingDeal(abi.DealID(dealID))
		if err != nil {
			return err
		}

		// Checks if allocation is in verifreg
		allocation, found, err := verifregActorV9.GetAllocation(proposal.Client, allocationId)
		if !found {
			return xerrors.Errorf("allocation %d not found for address %s", allocationId, proposal.Client)
		}
		if err != nil {
			return err
		}

		err = compareProposalToAllocation(proposal, *allocation)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("Pending Verified deals in market v8: %d\n", numPendingVerifiedDeals)

	numAllocationIds, err := countAllocationIds(actorStore, marketStateV9)
	if err != nil {
		return err
	}
	fmt.Printf("Allocation IDs in market v9: %d\n", numAllocationIds)

	if numAllocationIds != numPendingVerifiedDeals {
		return xerrors.Errorf("number of allocation IDsf: %d did not match the number of pending verified deals: %d", numAllocationIds, numPendingVerifiedDeals)
	}

	numAllocations, err := countAllocations(verifregStateV9, actorStore)
	if err != nil {
		return err
	}
	fmt.Printf("Allocations in verifreg v9: %d\n", numAllocations)

	if numAllocations != numPendingVerifiedDeals {
		return xerrors.Errorf("number of allocations: %d did not match the number of pending verified deals: %d", numAllocations, numPendingVerifiedDeals)
	}

	nextAllocationId := int(verifregStateV9.NextAllocationId)
	fmt.Printf("Next Allocation ID: %d\n", nextAllocationId)

	if numAllocations+1 != nextAllocationId {
		return xerrors.Errorf("number of allocations + 1: %d did not match the next allocation ID: %d", numAllocations+1, nextAllocationId)
	}

	return nil
}

func compareProposalToAllocation(prop market8.DealProposal, alloc verifreg9.Allocation) error {
	if prop.PieceCID != alloc.Data {
		return xerrors.Errorf("piece cid mismatch between proposal and allocation: %s, %s", prop.PieceCID, alloc.Data)
	}

	proposalClientID, err := address.IDFromAddress(prop.Client)
	if err != nil {
		return xerrors.Errorf("couldnt get ID from address")
	}
	if proposalClientID != uint64(alloc.Client) {
		return xerrors.Errorf("client id mismatch between proposal and allocation: %v, %v", proposalClientID, alloc.Client)
	}

	proposalProviderID, err := address.IDFromAddress(prop.Provider)
	if err != nil {
		return xerrors.Errorf("couldnt get ID from address")
	}
	if proposalProviderID != uint64(alloc.Provider) {
		return xerrors.Errorf("provider id mismatch between proposal and allocation: %v, %v", proposalProviderID, alloc.Provider)
	}

	if prop.PieceSize != alloc.Size {
		return xerrors.Errorf("piece size mismatch between proposal and allocation: %v, %v", prop.PieceSize, alloc.Size)
	}

	if alloc.TermMax != 540*builtin.EpochsInDay {
		return xerrors.Errorf("allocation term should be 540 days. Got %d epochs", alloc.TermMax)
	}

	if prop.EndEpoch-prop.StartEpoch != alloc.TermMin {
		return xerrors.Errorf("allocation term mismatch between proposal and allocation: %d, %d", prop.EndEpoch-prop.StartEpoch, alloc.TermMin)
	}

	return nil
}

func getMarketStateV8(stateTreeV8 *state.StateTree, actorStore adt.Store) (market8.State, error) {
	marketV8, err := stateTreeV8.GetActor(market.Address)
	if err != nil {
		return market8.State{}, err
	}

	var marketStateV8 market8.State
	if err = actorStore.Get(actorStore.Context(), marketV8.Head, &marketStateV8); err != nil {
		return market8.State{}, xerrors.Errorf("failed to get market actor state: %w", err)
	}

	return marketStateV8, nil
}

func getMarketStateV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (market9.State, error) {
	marketV9, err := stateTreeV9.GetActor(market.Address)
	if err != nil {
		return market9.State{}, err
	}

	var marketStateV9 market9.State
	if err = actorStore.Get(actorStore.Context(), marketV9.Head, &marketStateV9); err != nil {
		return market9.State{}, xerrors.Errorf("failed to get market actor state: %w", err)
	}

	return marketStateV9, nil
}

func getMarketActorV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (market.State, error) {
	marketV9, err := stateTreeV9.GetActor(market.Address)
	if err != nil {
		return nil, err
	}

	return market.Load(actorStore, marketV9)
}

func getVerifregActorV8(stateTreeV8 *state.StateTree, actorStore adt.Store) (verifreg.State, error) {
	verifregV8, err := stateTreeV8.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	return verifreg.Load(actorStore, verifregV8)
}

func getVerifregActorV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (verifreg.State, error) {
	verifregV9, err := stateTreeV9.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	return verifreg.Load(actorStore, verifregV9)
}

func getVerifregStateV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (verifreg9.State, error) {
	verifregV9, err := stateTreeV9.GetActor(verifreg.Address)
	if err != nil {
		return verifreg9.State{}, err
	}

	var verifregStateV9 verifreg9.State
	if err = actorStore.Get(actorStore.Context(), verifregV9.Head, &verifregStateV9); err != nil {
		return verifreg9.State{}, xerrors.Errorf("failed to get verifreg actor state: %w", err)
	}

	return verifregStateV9, nil
}

func getDatacapActorV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (datacap.State, error) {
	datacapV9, err := stateTreeV9.GetActor(datacap.Address)
	if err != nil {
		return nil, err
	}

	return datacap.Load(actorStore, datacapV9)
}

func checkAllMinersUnsealedCID(stateTreeV9 *state.StateTree, store adt.Store) error {
	return stateTreeV9.ForEach(func(addr address.Address, actor *types.Actor) error {
		if !lbuiltin.IsStorageMinerActor(actor.Code) {
			return nil // no need to check
		}

		err := checkMinerUnsealedCID(actor, stateTreeV9, store)
		if err != nil {
			fmt.Println("failure for miner ", addr)
			return err
		}
		return nil
	})
}

func checkMinerUnsealedCID(act *types.Actor, stateTreeV9 *state.StateTree, store adt.Store) error {
	minerCodeCid, found := actors.GetActorCodeID(actorstypes.Version9, manifest.MinerKey)
	if !found {
		return xerrors.Errorf("could not find code cid for miner actor")
	}
	if minerCodeCid != act.Code {
		return nil // no need to check
	}

	marketActorV9, err := getMarketActorV9(stateTreeV9, store)
	if err != nil {
		return err
	}
	dealProposals, err := marketActorV9.Proposals()
	if err != nil {
		return err
	}

	m, err := miner.Load(store, act)
	if err != nil {
		return err
	}

	err = m.ForEachPrecommittedSector(func(info miner9.SectorPreCommitOnChainInfo) error {
		dealIDs := info.Info.DealIDs

		if len(dealIDs) == 0 {
			return nil // Nothing to check here
		}

		pieceCids := make([]abi.PieceInfo, len(dealIDs))
		for i, dealId := range dealIDs {
			dealProposal, found, err := dealProposals.Get(dealId)
			if err != nil {
				return err
			}
			if !found {
				return nil
			}

			pieceCids[i] = abi.PieceInfo{
				Size:     dealProposal.PieceSize,
				PieceCID: dealProposal.PieceCID,
			}
		}

		if len(pieceCids) == 0 {
			return nil
		}

		if info.Info.UnsealedCid == nil {
			return xerrors.Errorf("nil unsealed CID for sector with deals")
		}

		pieceCID, err := ffi.GenerateUnsealedCID(abi.RegisteredSealProof_StackedDrg64GiBV1_1, pieceCids)
		if err != nil {
			return err
		}

		if pieceCID != *info.Info.UnsealedCid {
			return xerrors.Errorf("calculated piece CID %s did not match unsealed CID in precommitted sector info: %s", pieceCID, *info.Info.UnsealedCid)
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func countAllocations(verifregState verifreg9.State, store adt.Store) (int, error) {
	var count = 0

	actorToHamtMap, err := adt9.AsMap(store, verifregState.Allocations, builtin.DefaultHamtBitwidth)
	if err != nil {
		return 0, xerrors.Errorf("couldn't get outer map: %x", err)
	}

	var innerHamtCid cbg.CborCid
	err = actorToHamtMap.ForEach(&innerHamtCid, func(key string) error {
		innerMap, err := adt9.AsMap(store, cid.Cid(innerHamtCid), builtin.DefaultHamtBitwidth)
		if err != nil {
			return xerrors.Errorf("couldn't get outer map: %x", err)
		}

		var allocation verifreg9.Allocation
		err = innerMap.ForEach(&allocation, func(key string) error {
			count++
			return nil
		})
		if err != nil {
			return xerrors.Errorf("couldn't iterate over inner map: %x", err)
		}

		return nil
	})
	if err != nil {
		return 0, xerrors.Errorf("couldn't iterate over outer map: %x", err)
	}

	return count, nil
}

func countAllocationIds(store adt.Store, marketState market9.State) (int, error) {
	allocationIds, err := adt9.AsMap(store, marketState.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
	if err != nil {
		return 0, err
	}

	var numAllocationIds int
	_ = allocationIds.ForEach(nil, func(key string) error {
		numAllocationIds++
		return nil
	})

	return numAllocationIds, nil
}
