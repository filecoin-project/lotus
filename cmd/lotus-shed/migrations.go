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

func checkStateInvariants(ctx context.Context, oldStateRoot cid.Cid, newStateRoot cid.Cid, bs blockstore.Blockstore) error {
	actorStore := store.ActorStore(ctx, blockstore.NewTieredBstore(bs, blockstore.NewMemorySync()))

	verifregDatacaps, err := getVerifreg8Datacaps(ctx, oldStateRoot, actorStore)
	if err != nil {
		return err
	}

	newDatacaps, err := getDatacap9Datacaps(ctx, newStateRoot, actorStore)
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

func getVerifreg8Datacaps(ctx context.Context, v8StateRoot cid.Cid, actorStore adt.Store) (map[address.Address]abi.StoragePower, error) {
	stateTreeV8, err := state.LoadStateTree(actorStore, v8StateRoot)
	if err != nil {
		return nil, err
	}

	verifregV8, err := stateTreeV8.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	verifregV8State, err := verifreg.Load(actorStore, verifregV8)
	if err = actorStore.Get(ctx, verifregV8.Head, &verifregV8State); err != nil {
		return nil, xerrors.Errorf("failed to get verifreg actor state: %w", err)
	}

	var verifregDatacaps = make(map[address.Address]abi.StoragePower)
	err = verifregV8State.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
		verifregDatacaps[addr] = dcap
		return nil
	})
	if err != nil {
		return nil, err
	}

	return verifregDatacaps, nil
}

func getDatacap9Datacaps(ctx context.Context, v9StateRoot cid.Cid, actorStore adt.Store) (map[address.Address]abi.StoragePower, error) {
	stateTreeV9, err := state.LoadStateTree(actorStore, v9StateRoot)
	if err != nil {
		return nil, err
	}

	datacapV9, err := stateTreeV9.GetActor(datacap.Address)
	if err != nil {
		return nil, err
	}

	datacapV9State, err := datacap.Load(actorStore, datacapV9)
	if err = actorStore.Get(ctx, datacapV9.Head, &datacapV9State); err != nil {
		return nil, xerrors.Errorf("failed to get verifreg actor state: %w", err)
	}

	var datacaps = make(map[address.Address]abi.StoragePower)
	err = datacapV9State.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
		datacaps[addr] = dcap
		return nil
	})
	if err != nil {
		return nil, err
	}

	return datacaps, nil
}
