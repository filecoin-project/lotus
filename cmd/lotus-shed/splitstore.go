package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v2"
	logging "github.com/ipfs/go-log"
	"github.com/urfave/cli/v2"

	badgerbs "github.com/filecoin-project/lotus/blockstore/badger"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
)

var splitstoreCmd = &cli.Command{
	Name:        "splitstore",
	Description: "manage the splitstore directly",
	Subcommands: []*cli.Command{
		splitstoreGCCmd,
	},
}

var splitstoreGCCmd = &cli.Command{
	Name:        "gc",
	Description: "compacts and garbage collects the hotstore; node must be offline",
	Action: func(cctx *cli.Context) error {
		logging.SetLogLevel("badger", "ERROR")   // nolint:errcheck
		logging.SetLogLevel("badgerbs", "ERROR") // nolint:errcheck

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		c, err := lkrepo.Config()
		if err != nil {
			return err
		}

		cfg, ok := c.(*config.FullNode)
		if !ok {
			return fmt.Errorf("bad config")
		}

		if !cfg.Chainstore.EnableSplitstore {
			return fmt.Errorf("splitstore is not enabled")
		}

		if cfg.Chainstore.Splitstore.HotStoreType != "badger" {
			return fmt.Errorf("hotstore is not badger; can't compact")
		}

		path, err := lkrepo.SplitstorePath()
		if err != nil {
			return err
		}

		path = filepath.Join(path, "hot.badger")

		opts, err := repo.BadgerBlockstoreOptions(repo.HotBlockstore, path, false)
		if err != nil {
			return err
		}

		bs, err := badgerbs.Open(opts)
		if err != nil {
			return err
		}

		defer bs.Close() //nolint:errcheck

		log.Infof("compacting hotstore")
		startCompact := time.Now()
		err = bs.Compact()
		if err != nil {
			log.Warnf("error compacting hotstore: %s", err)
			return err
		}
		log.Infow("hotstore compaction done", "took", time.Since(startCompact))

		log.Infof("garbage collecting hotstore")
		startGC := time.Now()

		size, err := bs.Size()
		if err != nil {
			return err
		}

		log.Infof("hotstore size is %d bytes", size)
		for {
			// gc more aggressively than CollectGarbage
			for err == nil {
				err = bs.DB.RunValueLogGC(0.025)
			}

			if err != badger.ErrNoRewrite {
				return err
			}

			err = bs.Compact()
			if err != nil {
				return err
			}

			newsize, err := bs.Size()
			if err != nil {
				return err
			}

			if newsize < size {
				log.Infof("reclaimed %d bytes", size-newsize)
				size = newsize
				continue
			}

			break
		}

		log.Infow("hotstore garbage collection done", "took", time.Since(startGC))

		return nil
	},
}
