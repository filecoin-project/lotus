package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	market8 "github.com/filecoin-project/go-state-types/builtin/v8/market"
	adt8 "github.com/filecoin-project/go-state-types/builtin/v8/util/adt"
	market9 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	adt9 "github.com/filecoin-project/go-state-types/builtin/v9/util/adt"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/specs-actors/v7/actors/migration/nv15"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
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
	Name:        "migrate-nv17",
	Description: "Run the nv17 migration",
	ArgsUsage:   "[block to look back from]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		blkCid, err := cid.Decode(cctx.Args().First())
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

		sm, err := stmgr.NewStateManager(cs, filcns.NewTipSetExecutor(), vm.Syscalls(ffiwrapper.ProofVerifier), filcns.DefaultUpgradeSchedule(), nil)
		if err != nil {
			return err
		}

		cache := nv15.NewMemMigrationCache()

		blk, err := cs.GetBlock(ctx, blkCid)
		if err != nil {
			return err
		}

		migrationTs, err := cs.LoadTipSet(ctx, types.NewTipSetKey(blk.Parents...))
		if err != nil {
			return err
		}

		ts1, err := cs.GetTipsetByHeight(ctx, blk.Height-180, migrationTs, false)
		if err != nil {
			return err
		}

		startTime := time.Now()

		err = filcns.PreUpgradeActorsV9(ctx, sm, cache, ts1.ParentState(), ts1.Height()-1, ts1)
		if err != nil {
			return err
		}

		fmt.Println("completed round 1, took ", time.Since(startTime))
		startTime = time.Now()

		newCid1, err := filcns.UpgradeActorsV9(ctx, sm, cache, nil, blk.ParentStateRoot, blk.Height-1, migrationTs)
		if err != nil {
			return err
		}
		fmt.Println("completed round actual (with cache), took ", time.Since(startTime))

		fmt.Println("new cid", newCid1)

		newCid2, err := filcns.UpgradeActorsV9(ctx, sm, nv15.NewMemMigrationCache(), nil, blk.ParentStateRoot, blk.Height-1, migrationTs)
		if err != nil {
			return err
		}
		fmt.Println("completed round actual (without cache), took ", time.Since(startTime))

		fmt.Println("new cid", newCid2)

		err = checkStateInvariants(ctx, blk.ParentStateRoot, newCid2, bs)
		if err != nil {
			return err
		}

		return nil
	},
}

func checkStateInvariants(ctx context.Context, v8StateRoot cid.Cid, v9StateRoot cid.Cid, bs blockstore.Blockstore) error {
	actorStore := store.ActorStore(ctx, blockstore.NewTieredBstore(bs, blockstore.NewMemorySync()))

	stateTreeV8, err := state.LoadStateTree(actorStore, v8StateRoot)
	if err != nil {
		return err
	}

	stateTreeV9, err := state.LoadStateTree(actorStore, v9StateRoot)
	if err != nil {
		return err
	}

	err = checkDatacaps(*stateTreeV8, *stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	err = checkPendingVerifiedDeals(*stateTreeV8, *stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	return nil
}

func checkDatacaps(stateTreeV8 state.StateTree, stateTreeV9 state.StateTree, actorStore adt.Store) error {
	verifregDatacaps, err := getVerifreg8Datacaps(stateTreeV8, actorStore)
	if err != nil {
		return err
	}

	newDatacaps, err := getDatacap9Datacaps(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	if len(verifregDatacaps) != len(newDatacaps) {
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

func getVerifreg8Datacaps(stateTreeV8 state.StateTree, actorStore adt.Store) (map[address.Address]abi.StoragePower, error) {
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

func getDatacap9Datacaps(stateTreeV9 state.StateTree, actorStore adt.Store) (map[address.Address]abi.StoragePower, error) {
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

func checkPendingVerifiedDeals(stateTreeV8 state.StateTree, stateTreeV9 state.StateTree, actorStore adt.Store) error {
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

		numPendingVerifiedDeals++
		// Checks if allocation ID is in market map
		allocationId, err := marketActorV9.GetAllocationIdForPendingDeal(abi.DealID(dealID))
		if err != nil {
			return err
		}
		// Checks if allocation is in verifreg
		_, found, err := verifregActorV9.GetAllocation(proposal.Client, allocationId)
		if !found {
			return xerrors.Errorf("allocation %d not found for address %s", allocationId, proposal.Client)
		}
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

	numAllocations, err := countAllocations(verifregActorV9)
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

func getMarketStateV8(stateTreeV8 state.StateTree, actorStore adt.Store) (market8.State, error) {
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

func getMarketStateV9(stateTreeV9 state.StateTree, actorStore adt.Store) (market9.State, error) {
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

func getMarketActorV9(stateTreeV9 state.StateTree, actorStore adt.Store) (market.State, error) {
	marketV9, err := stateTreeV9.GetActor(market.Address)
	if err != nil {
		return nil, err
	}

	return market.Load(actorStore, marketV9)
}

func getVerifregActorV8(stateTreeV8 state.StateTree, actorStore adt.Store) (verifreg.State, error) {
	verifregV8, err := stateTreeV8.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	return verifreg.Load(actorStore, verifregV8)
}

func getVerifregActorV9(stateTreeV9 state.StateTree, actorStore adt.Store) (verifreg.State, error) {
	verifregV9, err := stateTreeV9.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	return verifreg.Load(actorStore, verifregV9)
}

func getVerifregStateV9(stateTreeV9 state.StateTree, actorStore adt.Store) (verifreg9.State, error) {
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

func getDatacapActorV9(stateTreeV9 state.StateTree, actorStore adt.Store) (datacap.State, error) {
	datacapV9, err := stateTreeV9.GetActor(datacap.Address)
	if err != nil {
		return nil, err
	}

	return datacap.Load(actorStore, datacapV9)
}

func countAllocations(verifregState verifreg.State) (int, error) {
	var count int
	err := verifregState.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
		allocations, err := verifregState.GetAllocations(addr)
		if err != nil {
			return err
		}
		count += len(allocations)
		return nil
	})
	if err != nil {
		return 0, err
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
