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
	verifregStateV8, err := getVerifregV8State(stateTreeV8, actorStore)
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
	datacapStateV9, err := getDatacapV9State(stateTreeV9, actorStore)
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
	marketStateV8, err := getMarketV8State(stateTreeV8, actorStore)
	if err != nil {
		return err
	}

	marketStateV9, err := getMarketV9State(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	verifregStateV9, err := getVerifregV9State(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	v8DealStates, err := marketStateV8.States()
	if err != nil {
		return err
	}

	v8DealProposals, err := marketStateV8.Proposals()
	if err != nil {
		return err
	}

	var numPendingVerifiedDeals = 0
	err = v8DealStates.ForEach(func(id abi.DealID, ds market.DealState) error {
		// Proposal hasn't been activated yet
		if ds.SectorStartEpoch == -1 {
			proposal, _, err := v8DealProposals.Get(id)
			if err != nil {
				return err
			}

			// Verified deal
			if proposal.VerifiedDeal {
				numPendingVerifiedDeals++
				// Checks if allocation ID is in market map
				allocationId, err := marketStateV9.GetAllocationIdForPendingDeal(id)
				if err != nil {
					return err
				}
				// Checks if allocation is in verifreg
				_, found, err := verifregStateV9.GetAllocation(proposal.Client, allocationId)
				if !found {
					return xerrors.Errorf("allocation %d not found for address %s", allocationId, proposal.Client)
				}
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	numAllocations, err := countAllocations(verifregStateV9)
	if err != nil {
		return err
	}

	if numAllocations != numPendingVerifiedDeals {
		return xerrors.Errorf("number of allocations: %d did not match the number of pending verified deals: %d", numAllocations, numPendingVerifiedDeals)
	}

	return nil
}

func getMarketV8State(stateTreeV8 state.StateTree, actorStore adt.Store) (market.State, error) {
	marketV9, err := stateTreeV8.GetActor(market.Address)
	if err != nil {
		return nil, err
	}

	return market.Load(actorStore, marketV9)
}

func getMarketV9State(stateTreeV9 state.StateTree, actorStore adt.Store) (market.State, error) {
	marketV9, err := stateTreeV9.GetActor(market.Address)
	if err != nil {
		return nil, err
	}

	return market.Load(actorStore, marketV9)
}

func getVerifregV8State(stateTreeV8 state.StateTree, actorStore adt.Store) (verifreg.State, error) {
	verifregV8, err := stateTreeV8.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	return verifreg.Load(actorStore, verifregV8)
}

func getVerifregV9State(stateTreeV9 state.StateTree, actorStore adt.Store) (verifreg.State, error) {
	verifregV9, err := stateTreeV9.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	return verifreg.Load(actorStore, verifregV9)
}

func getDatacapV9State(stateTreeV9 state.StateTree, actorStore adt.Store) (datacap.State, error) {
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
