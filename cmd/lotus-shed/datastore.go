package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/docker/go-units"
	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log"
	"github.com/polydawn/refmt/cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

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
	},
}

var datastoreListCmd = &cli.Command{
	Name:        "list",
	Description: "list datastore keys",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "repo-type",
			Usage: "node type (1 - full, 2 - storage, 3 - worker)",
			Value: 1,
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
		logging.SetLogLevel("badger", "ERROR") // nolint:errcheck

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

		lr, err := r.Lock(repo.RepoType(cctx.Int("repo-type")))
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(datastore.NewKey(cctx.Args().First()).String())
		if err != nil {
			return err
		}

		genc := cctx.String("get-enc")

		q, err := ds.Query(dsq.Query{
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

var datastoreGetCmd = &cli.Command{
	Name:        "get",
	Description: "list datastore keys",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "repo-type",
			Usage: "node type (1 - full, 2 - storage, 3 - worker)",
			Value: 1,
		},
		&cli.StringFlag{
			Name:  "enc",
			Usage: "encoding (esc/hex/cbor)",
			Value: "esc",
		},
	},
	ArgsUsage: "[namespace key]",
	Action: func(cctx *cli.Context) error {
		logging.SetLogLevel("badger", "ERROR") // nolint:errchec

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

		lr, err := r.Lock(repo.RepoType(cctx.Int("repo-type")))
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(datastore.NewKey(cctx.Args().First()).String())
		if err != nil {
			return err
		}

		val, err := ds.Get(datastore.NewKey(cctx.Args().Get(1)))
		if err != nil {
			return xerrors.Errorf("get: %w", err)
		}

		return printVal(cctx.String("enc"), val)
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
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("expected 1 argument")
		}

		f, err := os.Open(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("opening backup file: %w", err)
		}
		defer f.Close() // nolint:errcheck

		var keys, kbytes, vbytes uint64
		err = backupds.ReadBackup(f, func(key datastore.Key, value []byte) error {
			keys++
			kbytes += uint64(len(key.String()))
			vbytes += uint64(len(value))
			return nil
		})
		if err != nil {
			return err
		}

		fmt.Println("Keys:        ", keys)
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
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("expected 1 argument")
		}

		f, err := os.Open(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("opening backup file: %w", err)
		}
		defer f.Close() // nolint:errcheck

		printKv := kvPrinter(cctx.Bool("top-level"), cctx.String("get-enc"))
		err = backupds.ReadBackup(f, func(key datastore.Key, value []byte) error {
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
