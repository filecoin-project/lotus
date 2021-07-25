package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dgraph-io/badger/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"go.uber.org/zap"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
)

var splitstoreCmd = &cli.Command{
	Name:        "splitstore",
	Description: "splitstore utilities",
	Subcommands: []*cli.Command{
		splitstoreRollbackCmd,
	},
}

var splitstoreRollbackCmd = &cli.Command{
	Name:        "rollback",
	Description: "rollbacks a splitstore installation",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.BoolFlag{
			Name:  "gc-coldstore",
			Usage: "compact and garbage collect the coldstore after copying the hotstore",
		},
		&cli.BoolFlag{
			Name:  "rewrite-config",
			Usage: "rewrite the lotus configuration to disable splitstore",
		},
	},
	Action: func(cctx *cli.Context) error {
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("error opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return xerrors.Errorf("error locking repo: %w", err)
		}
		defer lr.Close() //nolint:errcheck

		cfg, err := lr.Config()
		if err != nil {
			return xerrors.Errorf("error getting config: %w", err)
		}

		fncfg, ok := cfg.(*config.FullNode)
		if !ok {
			return xerrors.Errorf("wrong config type: %T", cfg)
		}

		if !fncfg.Chainstore.EnableSplitstore {
			return xerrors.Errorf("splitstore is not enabled")
		}

		fmt.Println("copying hotstore to coldstore...")
		err = copyHotstoreToColdstore(lr, cctx.Bool("gc-coldstore"))
		if err != nil {
			return xerrors.Errorf("error copying hotstore to coldstore: %w", err)
		}

		fmt.Println("deleting splitstore directory...")
		err = deleteSplitstoreDir(lr)
		if err != nil {
			return xerrors.Errorf("error deleting splitstore directory: %w", err)
		}

		fmt.Println("deleting splitstore keys from metadata datastore...")
		err = deleteSplitstoreKeys(lr)
		if err != nil {
			return xerrors.Errorf("error deleting splitstore keys: %w", err)
		}

		if cctx.Bool("rewrite-config") {
			fmt.Println("disabling splitstore in config...")
			err = lr.SetConfig(func(cfg interface{}) {
				cfg.(*config.FullNode).Chainstore.EnableSplitstore = false
			})
			if err != nil {
				return xerrors.Errorf("error disabling splitstore in config: %w", err)
			}
		}

		fmt.Println("splitstore has been rolled back.")
		return nil
	},
}

func copyHotstoreToColdstore(lr repo.LockedRepo, gcColdstore bool) error {
	repoPath := lr.Path()
	dataPath := filepath.Join(repoPath, "datastore")
	coldPath := filepath.Join(dataPath, "chain")
	hotPath := filepath.Join(dataPath, "splitstore", "hot.badger")

	blog := &badgerLogger{
		SugaredLogger: log.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar(),
		skip2:         log.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
	}

	coldOpts, err := repo.BadgerBlockstoreOptions(repo.UniversalBlockstore, coldPath, false)
	if err != nil {
		return xerrors.Errorf("error getting coldstore badger options: %w", err)
	}
	coldOpts.SyncWrites = false
	coldOpts.Logger = blog

	hotOpts, err := repo.BadgerBlockstoreOptions(repo.HotBlockstore, hotPath, true)
	if err != nil {
		return xerrors.Errorf("error getting hotstore badger options: %w", err)
	}
	hotOpts.Logger = blog

	cold, err := badger.Open(coldOpts.Options)
	if err != nil {
		return xerrors.Errorf("error opening coldstore: %w", err)
	}
	defer cold.Close() //nolint

	hot, err := badger.Open(hotOpts.Options)
	if err != nil {
		return xerrors.Errorf("error opening hotstore: %w", err)
	}
	defer hot.Close() //nolint

	rd, wr := io.Pipe()
	g := new(errgroup.Group)

	g.Go(func() error {
		bwr := bufio.NewWriterSize(wr, 64<<20)

		_, err := hot.Backup(bwr, 0)
		if err != nil {
			_ = wr.CloseWithError(err)
			return err
		}

		err = bwr.Flush()
		if err != nil {
			_ = wr.CloseWithError(err)
			return err
		}

		return wr.Close()
	})

	g.Go(func() error {
		err := cold.Load(rd, 1024)
		if err != nil {
			return err
		}

		return cold.Sync()
	})

	err = g.Wait()
	if err != nil {
		return err
	}

	// compact + gc the coldstore if so requested
	if gcColdstore {
		fmt.Println("compacting coldstore...")
		nworkers := runtime.NumCPU()
		if nworkers < 2 {
			nworkers = 2
		}

		err = cold.Flatten(nworkers)
		if err != nil {
			return xerrors.Errorf("error compacting coldstore: %w", err)
		}

		fmt.Println("garbage collecting coldstore...")
		for err == nil {
			err = cold.RunValueLogGC(0.0625)
		}

		if err != badger.ErrNoRewrite {
			return xerrors.Errorf("error garbage collecting coldstore: %w", err)
		}
	}

	return nil
}

func deleteSplitstoreDir(lr repo.LockedRepo) error {
	path, err := lr.SplitstorePath()
	if err != nil {
		return xerrors.Errorf("error getting splitstore path: %w", err)
	}

	return os.RemoveAll(path)
}

func deleteSplitstoreKeys(lr repo.LockedRepo) error {
	ds, err := lr.Datastore(context.TODO(), "/metadata")
	if err != nil {
		return xerrors.Errorf("error opening datastore: %w", err)
	}
	if closer, ok := ds.(io.Closer); ok {
		defer closer.Close() //nolint
	}

	var keys []datastore.Key
	res, err := ds.Query(query.Query{Prefix: "/splitstore"})
	if err != nil {
		return xerrors.Errorf("error querying datastore for splitstore keys: %w", err)
	}

	for r := range res.Next() {
		if r.Error != nil {
			return xerrors.Errorf("datastore query error: %w", r.Error)
		}

		keys = append(keys, datastore.NewKey(r.Key))
	}

	for _, k := range keys {
		fmt.Printf("deleting %s from datastore...\n", k)
		err = ds.Delete(k)
		if err != nil {
			return xerrors.Errorf("error deleting key %s from datastore: %w", k, err)
		}
	}

	return nil
}

// badger logging through go-log
type badgerLogger struct {
	*zap.SugaredLogger
	skip2 *zap.SugaredLogger
}

func (b *badgerLogger) Warningf(format string, args ...interface{}) {}
func (b *badgerLogger) Infof(format string, args ...interface{})    {}
func (b *badgerLogger) Debugf(format string, args ...interface{})   {}
