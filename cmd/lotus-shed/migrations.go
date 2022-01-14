package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/v7/actors/migration/nv15"

	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

var migrationsCmd = &cli.Command{
	Name:        "migrate-nv15",
	Description: "Run the specified migration",
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
			return fmt.Errorf("must pass block cid")
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

		blk, err := cs.GetBlock(blkCid)
		if err != nil {
			return err
		}

		migrationTs, err := cs.LoadTipSet(types.NewTipSetKey(blk.Parents...))
		if err != nil {
			return err
		}

		ts1, err := cs.GetTipsetByHeight(ctx, blk.Height-240, migrationTs, false)
		if err != nil {
			return err
		}

		startTime := time.Now()

		err = filcns.PreUpgradeActorsV7(ctx, sm, cache, ts1.ParentState(), ts1.Height()-1, ts1)
		if err != nil {
			return err
		}

		fmt.Println("completed round 1, took ", time.Since(startTime))
		startTime = time.Now()

		newCid1, err := filcns.UpgradeActorsV7(ctx, sm, cache, nil, blk.ParentStateRoot, blk.Height-1, migrationTs)
		if err != nil {
			return err
		}
		fmt.Println("completed round actual (with cache), took ", time.Since(startTime))

		fmt.Println("new cid", newCid1)

		newCid2, err := filcns.UpgradeActorsV7(ctx, sm, nv15.NewMemMigrationCache(), nil, blk.ParentStateRoot, blk.Height-1, migrationTs)
		if err != nil {
			return err
		}
		fmt.Println("completed round actual (without cache), took ", time.Since(startTime))

		fmt.Println("new cid", newCid2)
		return nil
	},
}
