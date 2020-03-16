package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-sectorbuilder"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/storage/sealmgr/stores"
)

const metaFile = "sectorstore.json"

var storageCmd = &cli.Command{
	Name:  "storage",
	Usage: "manage sector storage",
	Subcommands: []*cli.Command{
		storageAttachCmd,
		storageListCmd,
	},
}

var storageAttachCmd = &cli.Command{
	Name:  "attach",
	Usage: "attach local storage path",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "init",
			Usage: "initialize the path first",
		},
		&cli.Uint64Flag{
			Name:  "weight",
			Usage: "(for init) path weight",
			Value: 10,
		},
		&cli.BoolFlag{
			Name:  "seal",
			Usage: "(for init) use path for sealing",
		},
		&cli.BoolFlag{
			Name:  "store",
			Usage: "(for init) use path for long-term storage",
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if !cctx.Args().Present() {
			return xerrors.Errorf("must specify storage path to attach")
		}

		p, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("expanding path: %w", err)
		}

		if cctx.Bool("init") {
			if err := os.MkdirAll(p, 0755); err != nil {
				if !os.IsExist(err) {
					return err
				}
			}

			_, err := os.Stat(filepath.Join(p, metaFile))
			if !os.IsNotExist(err) {
				if err == nil {
					return xerrors.Errorf("path is already initialized")
				}
				return err
			}

			cfg := &stores.StorageMeta{
				ID:       stores.ID(uuid.New().String()),
				Weight:   cctx.Uint64("weight"),
				CanSeal:  cctx.Bool("seal"),
				CanStore: cctx.Bool("store"),
			}

			if !(cfg.CanStore || cfg.CanSeal) {
				return xerrors.Errorf("must specify at least one of --store of --seal")
			}

			b, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return xerrors.Errorf("marshaling storage config: %w", err)
			}

			if err := ioutil.WriteFile(filepath.Join(p, metaFile), b, 0644); err != nil {
				return xerrors.Errorf("persisting storage metadata (%s): %w", filepath.Join(p, metaFile), err)
			}
		}

		return nodeApi.StorageAddLocal(ctx, p)
	},
}

var storageListCmd = &cli.Command{
	Name: "list",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		st, err := nodeApi.StorageList(ctx)
		if err != nil {
			return err
		}

		for id, sectors := range st {
			var u, s, c int
			for _, decl := range sectors {
				if decl.SectorFileType&sectorbuilder.FTUnsealed > 0 {
					u++
				}
				if decl.SectorFileType&sectorbuilder.FTSealed > 0 {
					s++
				}
				if decl.SectorFileType&sectorbuilder.FTCache > 0 {
					c++
				}
			}

			fmt.Printf("%s:\n", id)
			fmt.Printf("\tUnsealed: %d; Sealed: %d; Caches: %d\n", u, s, c)

			si, err := nodeApi.StorageInfo(ctx, id)
			if err != nil {
				return err
			}
			fmt.Printf("\tSeal: %t; Store: %t; Cost: %d\n", si.CanSeal, si.CanStore, si.Cost)
			for _, l := range si.URLs {
				fmt.Printf("\tReachable %s\n", l) // TODO; try pinging maybe?? print latency?
			}
		}

		return nil
	},
}
