package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dgraph-io/badger/v2"
	"github.com/docker/go-units"
	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/polydawn/refmt/cbor"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-shed/shedgen"
	"github.com/filecoin-project/lotus/lib/backupds"
	"github.com/filecoin-project/lotus/node/repo"
)

var datastoreCmd = &cli.Command{
	Name:        "datastore",
	Description: "access node datastores directly",
	Subcommands: []*cli.Command{
		datastoreBackupCmd,
		datastoreListCmd,
		datastoreGetCmd,
		datastoreRewriteCmd,
		datastoreVlog2CarCmd,
		datastoreImportCmd,
		datastoreExportCmd,
		datastoreClearCmd,
	},
}

var datastoreListCmd = &cli.Command{
	Name:        "list",
	Description: "list datastore keys",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo-type",
			Usage: "node type (FullNode, StorageMiner, Worker, Wallet)",
			Value: "FullNode",
		},
		&cli.BoolFlag{
			Name:  "top-level",
			Usage: "only print top-level keys",
		},
		&cli.StringFlag{
			Name:  "get-enc",
			Usage: "print values [esc/hex/cbor]",
		},
	},
	ArgsUsage: "[namespace prefix]",
	Action: func(cctx *cli.Context) error {
		_ = logging.SetLogLevel("badger", "ERROR")

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.NewRepoTypeFromString(cctx.String("repo-type")))
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(context.Background(), datastore.NewKey(cctx.Args().First()).String())
		if err != nil {
			return err
		}

		genc := cctx.String("get-enc")

		q, err := ds.Query(context.Background(), dsq.Query{
			Prefix:   datastore.NewKey(cctx.Args().Get(1)).String(),
			KeysOnly: genc == "",
		})
		if err != nil {
			return xerrors.Errorf("datastore query: %w", err)
		}
		defer q.Close() //nolint:errcheck

		printKv := kvPrinter(cctx.Bool("top-level"), genc)

		for res := range q.Next() {
			if err := printKv(res.Key, res.Value); err != nil {
				return err
			}
		}

		return nil
	},
}

var datastoreClearCmd = &cli.Command{
	Name:        "clear",
	Description: "Clear a part or all of the given datastore.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo-type",
			Usage: "node type (FullNode, StorageMiner, Worker, Wallet)",
			Value: "FullNode",
		},
		&cli.StringFlag{
			Name:  "prefix",
			Usage: "only delete key/values with the given prefix",
			Value: "",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "must be specified for the action to take effect",
		},
	},
	ArgsUsage: "[namespace]",
	Action: func(cctx *cli.Context) (_err error) {
		if cctx.NArg() != 1 {
			return xerrors.Errorf("requires 1 argument: the datastore prefix")
		}
		namespace := cctx.Args().Get(0)

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.NewRepoTypeFromString(cctx.String("repo-type")))
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(cctx.Context, datastore.NewKey(namespace).String())
		if err != nil {
			return err
		}
		defer func() {
			_err = multierr.Append(_err, ds.Close())
		}()

		dryRun := !cctx.Bool("really-do-it")

		query, err := ds.Query(cctx.Context, dsq.Query{
			Prefix: cctx.String("prefix"),
		})
		if err != nil {
			return err
		}
		defer query.Close() //nolint:errcheck

		batch, err := ds.Batch(cctx.Context)
		if err != nil {
			return xerrors.Errorf("failed to create a datastore batch: %w", err)
		}

		for res, ok := query.NextSync(); ok; res, ok = query.NextSync() {
			if res.Error != nil {
				return xerrors.Errorf("failed to read from datastore: %w", res.Error)
			}
			_, _ = fmt.Fprintf(cctx.App.Writer, "deleting: %q\n", res.Key)
			if !dryRun {
				if err := batch.Delete(cctx.Context, datastore.NewKey(res.Key)); err != nil {
					return xerrors.Errorf("failed to delete %q: %w", res.Key, err)
				}
			}
		}

		if !dryRun {
			if err := batch.Commit(cctx.Context); err != nil {
				return xerrors.Errorf("failed to flush the batch: %w", err)
			}
		} else {
			_, _ = fmt.Fprintln(cctx.App.Writer, "NOTE: dry run complete, re-run with --really-do-it to actually delete this state.")
		}

		return nil
	},
}

var datastoreGetCmd = &cli.Command{
	Name:        "get",
	Description: "list datastore keys",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo-type",
			Usage: "node type (FullNode, StorageMiner, Worker, Wallet)",
			Value: "FullNode",
		},
		&cli.StringFlag{
			Name:  "enc",
			Usage: "encoding (esc/hex/cbor)",
			Value: "esc",
		},
	},
	ArgsUsage: "[namespace key]",
	Action: func(cctx *cli.Context) error {
		_ = logging.SetLogLevel("badger", "ERROR")

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.NewRepoTypeFromString(cctx.String("repo-type")))
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(context.Background(), datastore.NewKey(cctx.Args().First()).String())
		if err != nil {
			return err
		}

		val, err := ds.Get(context.Background(), datastore.NewKey(cctx.Args().Get(1)))
		if err != nil {
			return xerrors.Errorf("get: %w", err)
		}

		return printVal(cctx.String("enc"), val)
	},
}

var datastoreExportCmd = &cli.Command{
	Name:        "export",
	Description: "Export part or all of the specified datastore, appending to the specified datastore snapshot.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo-type",
			Usage: "node type (FullNode, StorageMiner, Worker, Wallet)",
			Value: "FullNode",
		},
		&cli.StringFlag{
			Name:  "prefix",
			Usage: "export only keys with the given prefix",
			Value: "",
		},
	},
	ArgsUsage: "[namespace filename]",
	Action: func(cctx *cli.Context) (_err error) {
		if cctx.NArg() != 2 {
			return xerrors.Errorf("requires 2 arguments: the datastore prefix and the filename to which the snapshot will be written")
		}
		namespace := cctx.Args().Get(0)
		fname := cctx.Args().Get(1)

		snapshot, err := os.OpenFile(fname, os.O_WRONLY|os.O_APPEND|os.O_CREATE, os.ModePerm)
		if err != nil {
			return xerrors.Errorf("failed to open snapshot: %w", err)
		}
		defer func() {
			_err = multierr.Append(_err, snapshot.Close())
		}()

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.NewRepoTypeFromString(cctx.String("repo-type")))
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(cctx.Context, datastore.NewKey(namespace).String())
		if err != nil {
			return err
		}
		defer func() {
			_err = multierr.Append(_err, ds.Close())
		}()

		query, err := ds.Query(cctx.Context, dsq.Query{
			Prefix: cctx.String("prefix"),
		})
		if err != nil {
			return err
		}

		bufWriter := bufio.NewWriter(snapshot)
		snapshotWriter := cbg.NewCborWriter(bufWriter)
		for res, ok := query.NextSync(); ok; res, ok = query.NextSync() {
			if res.Error != nil {
				return xerrors.Errorf("failed to read from datastore: %w", res.Error)
			}

			entry := shedgen.DatastoreEntry{
				Key:   []byte(res.Key),
				Value: res.Value,
			}

			_, _ = fmt.Fprintf(cctx.App.Writer, "exporting: %q\n", res.Key)
			if err := entry.MarshalCBOR(snapshotWriter); err != nil {
				return xerrors.Errorf("failed to write %q to snapshot: %w", res.Key, err)
			}
		}
		if err := bufWriter.Flush(); err != nil {
			return xerrors.Errorf("failed to flush snapshot: %w", err)
		}

		return nil
	},
}

var datastoreImportCmd = &cli.Command{
	Name: "import",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo-type",
			Usage: "node type (FullNode, StorageMiner, Worker, Wallet)",
			Value: "FullNode",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "must be specified for the action to take effect",
		},
	},
	Description: "Import the specified datastore snapshot.",
	ArgsUsage:   "[namespace filename]",
	Action: func(cctx *cli.Context) (_err error) {
		if cctx.NArg() != 2 {
			return xerrors.Errorf("requires 2 arguments: the datastore prefix and the filename of the snapshot to import")
		}
		namespace := cctx.Args().Get(0)
		fname := cctx.Args().Get(1)

		snapshot, err := os.Open(fname)
		if err != nil {
			return xerrors.Errorf("failed to open snapshot: %w", err)
		}
		defer snapshot.Close() //nolint:errcheck

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.NewRepoTypeFromString(cctx.String("repo-type")))
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(cctx.Context, datastore.NewKey(namespace).String())
		if err != nil {
			return err
		}
		defer func() {
			_err = multierr.Append(_err, ds.Close())
		}()

		batch, err := ds.Batch(cctx.Context)
		if err != nil {
			return err
		}

		dryRun := !cctx.Bool("really-do-it")

		snapshotReader := cbg.NewCborReader(bufio.NewReader(snapshot))
		for {
			var entry shedgen.DatastoreEntry
			if err := entry.UnmarshalCBOR(snapshotReader); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return xerrors.Errorf("failed to read entry from snapshot: %w", err)
			}

			_, _ = fmt.Fprintf(cctx.App.Writer, "importing: %q\n", string(entry.Key))

			if !dryRun {
				key := datastore.NewKey(string(entry.Key))
				if err := batch.Put(cctx.Context, key, entry.Value); err != nil {
					return xerrors.Errorf("failed to put %q: %w", key, err)
				}
			}
		}

		if !dryRun {
			if err := batch.Commit(cctx.Context); err != nil {
				return xerrors.Errorf("failed to commit batch: %w", err)
			}
		} else {
			_, _ = fmt.Fprintln(cctx.App.Writer, "NOTE: dry run complete, re-run with --really-do-it to actually import the datastore snapshot, overwriting any conflicting state.")
		}
		return nil
	},
}

var datastoreBackupCmd = &cli.Command{
	Name:        "backup",
	Description: "manage datastore backups",
	Subcommands: []*cli.Command{
		datastoreBackupStatCmd,
		datastoreBackupListCmd,
	},
}

var datastoreBackupStatCmd = &cli.Command{
	Name:        "stat",
	Description: "validate and print info about datastore backup",
	ArgsUsage:   "[file]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		f, err := os.Open(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("opening backup file: %w", err)
		}
		defer f.Close() // nolint:errcheck

		var keys, logs, kbytes, vbytes uint64
		clean, err := backupds.ReadBackup(f, func(key datastore.Key, value []byte, log bool) error {
			if log {
				logs++
			}
			keys++
			kbytes += uint64(len(key.String()))
			vbytes += uint64(len(value))
			return nil
		})
		if err != nil {
			return err
		}

		fmt.Println("Truncated:   ", !clean)
		fmt.Println("Keys:        ", keys)
		fmt.Println("Log values:  ", log)
		fmt.Println("Key bytes:   ", units.BytesSize(float64(kbytes)))
		fmt.Println("Value bytes: ", units.BytesSize(float64(vbytes)))

		return err
	},
}

var datastoreBackupListCmd = &cli.Command{
	Name:        "list",
	Description: "list data in a backup",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "top-level",
			Usage: "only print top-level keys",
		},
		&cli.StringFlag{
			Name:  "get-enc",
			Usage: "print values [esc/hex/cbor]",
		},
	},
	ArgsUsage: "[file]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		f, err := os.Open(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("opening backup file: %w", err)
		}
		defer f.Close() // nolint:errcheck

		printKv := kvPrinter(cctx.Bool("top-level"), cctx.String("get-enc"))
		_, err = backupds.ReadBackup(f, func(key datastore.Key, value []byte, _ bool) error {
			return printKv(key.String(), value)
		})
		if err != nil {
			return err
		}

		return err
	},
}

func kvPrinter(toplevel bool, genc string) func(sk string, value []byte) error {
	seen := map[string]struct{}{}

	return func(s string, value []byte) error {
		if toplevel {
			k := datastore.NewKey(datastore.NewKey(s).List()[0])
			if k.Type() != "" {
				s = k.Type()
			} else {
				s = k.String()
			}

			_, has := seen[s]
			if has {
				return nil
			}
			seen[s] = struct{}{}
		}

		s = fmt.Sprintf("%q", s)
		s = strings.Trim(s, "\"")
		fmt.Println(s)

		if genc != "" {
			fmt.Print("\t")
			if err := printVal(genc, value); err != nil {
				return err
			}
		}

		return nil
	}
}

func printVal(enc string, val []byte) error {
	switch enc {
	case "esc":
		s := fmt.Sprintf("%q", string(val))
		s = strings.Trim(s, "\"")
		fmt.Println(s)
	case "hex":
		fmt.Printf("%x\n", val)
	case "cbor":
		var out interface{}
		if err := cbor.Unmarshal(cbor.DecodeOptions{}, val, &out); err != nil {
			return xerrors.Errorf("unmarshaling cbor: %w", err)
		}
		s, err := json.Marshal(&out)
		if err != nil {
			return xerrors.Errorf("remarshaling as json: %w", err)
		}

		fmt.Println(string(s))
	default:
		return xerrors.New("unknown encoding")
	}

	return nil
}

var datastoreRewriteCmd = &cli.Command{
	Name:        "rewrite",
	Description: "rewrites badger datastore to compact it and possibly change params",
	ArgsUsage:   "source destination",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}
		fromPath, err := homedir.Expand(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("cannot get fromPath: %w", err)
		}
		toPath, err := homedir.Expand(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("cannot get toPath: %w", err)
		}

		var (
			from *badger.DB
			to   *badger.DB
		)

		// open the destination (to) store.
		opts, err := repo.BadgerBlockstoreOptions(repo.UniversalBlockstore, toPath, false)
		if err != nil {
			return xerrors.Errorf("failed to get badger options: %w", err)
		}
		opts.SyncWrites = false
		if to, err = badger.Open(opts.Options); err != nil {
			return xerrors.Errorf("opening 'to' badger store: %w", err)
		}

		// open the source (from) store.
		opts, err = repo.BadgerBlockstoreOptions(repo.UniversalBlockstore, fromPath, true)
		if err != nil {
			return xerrors.Errorf("failed to get badger options: %w", err)
		}
		if from, err = badger.Open(opts.Options); err != nil {
			return xerrors.Errorf("opening 'from' datastore: %w", err)
		}

		pr, pw := io.Pipe()
		errCh := make(chan error)
		go func() {
			bw := bufio.NewWriterSize(pw, 64<<20)
			_, err := from.Backup(bw, 0)
			_ = bw.Flush()
			_ = pw.CloseWithError(err)
			errCh <- err
		}()
		go func() {
			err := to.Load(pr, 256)
			errCh <- err
		}()

		err = <-errCh
		if err != nil {
			select {
			case nerr := <-errCh:
				err = multierr.Append(err, nerr)
			default:
			}
			return err
		}

		err = <-errCh
		if err != nil {
			return err
		}
		return multierr.Append(from.Close(), to.Close())
	},
}
