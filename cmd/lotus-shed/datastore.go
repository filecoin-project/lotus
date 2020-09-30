package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log"
	"github.com/polydawn/refmt/cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/repo"
)

var datastoreCmd = &cli.Command{
	Name:        "datastore",
	Description: "access node datastores directly",
	Subcommands: []*cli.Command{
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

		genc := cctx.String("get-enc")

		q, err := ds.Query(dsq.Query{
			Prefix:   datastore.NewKey(cctx.Args().Get(1)).String(),
			KeysOnly: genc == "",
		})
		if err != nil {
			return xerrors.Errorf("datastore query: %w", err)
		}
		defer q.Close() //nolint:errcheck

		seen := map[string]struct{}{}
		for res := range q.Next() {
			s := res.Key
			if cctx.Bool("top-level") {
				k := datastore.NewKey(datastore.NewKey(s).List()[0])
				if k.Type() != "" {
					s = k.Type()
				} else {
					s = k.String()
				}

				_, has := seen[s]
				if has {
					continue
				}
				seen[s] = struct{}{}
			}

			s = fmt.Sprintf("%q", s)
			s = strings.Trim(s, "\"")
			fmt.Println(s)

			if genc != "" {
				fmt.Print("\t")
				if err := printVal(genc, res.Value); err != nil {
					return err
				}
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
