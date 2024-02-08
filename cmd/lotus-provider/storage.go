package main

import (
	"encoding/json"
	"github.com/docker/go-units"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/rpc"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"os"
	"path/filepath"
)

const metaFile = "sectorstore.json"

var storageCmd = &cli.Command{
	Name:  "storage",
	Usage: "manage sector storage",
	Description: `Sectors can be stored across many filesystem paths. These
commands provide ways to manage the storage the miner will used to store sectors
long term for proving (references as 'store') as well as how sectors will be
stored while moving through the sealing pipeline (references as 'seal').`,
	Subcommands: []*cli.Command{
		storageAttachCmd,
		/*storageDetachCmd,
		storageRedeclareCmd,
		storageListCmd,
		storageFindCmd,
		storageCleanupCmd,
		storageLocks,*/
	},
}

var storageAttachCmd = &cli.Command{
	Name:      "attach",
	Usage:     "attach local storage path",
	ArgsUsage: "[path]",
	Description: `Storage can be attached to the miner using this command. The storage volume
list is stored local to the miner in storage.json set in lotus-provider run. We do not
recommend manually modifying this value without further understanding of the
storage system.

Each storage volume contains a configuration file which describes the
capabilities of the volume. When the '--init' flag is provided, this file will
be created using the additional flags.

Weight
A high weight value means data will be more likely to be stored in this path

Seal
Data for the sealing process will be stored here

Store
Finalized sectors that will be moved here for long term storage and be proven
over time
   `,
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
		&cli.StringFlag{
			Name:  "max-storage",
			Usage: "(for init) limit storage space for sectors (expensive for very large paths!)",
		},
		&cli.StringSliceFlag{
			Name:  "groups",
			Usage: "path group names",
		},
		&cli.StringSliceFlag{
			Name:  "allow-to",
			Usage: "path groups allowed to pull data from this path (allow all if not specified)",
		},
	},
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := rpc.GetProviderAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
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

			var maxStor int64
			if cctx.IsSet("max-storage") {
				maxStor, err = units.RAMInBytes(cctx.String("max-storage"))
				if err != nil {
					return xerrors.Errorf("parsing max-storage: %w", err)
				}
			}

			cfg := &storiface.LocalStorageMeta{
				ID:         storiface.ID(uuid.New().String()),
				Weight:     cctx.Uint64("weight"),
				CanSeal:    cctx.Bool("seal"),
				CanStore:   cctx.Bool("store"),
				MaxStorage: uint64(maxStor),
				Groups:     cctx.StringSlice("groups"),
				AllowTo:    cctx.StringSlice("allow-to"),
			}

			if !(cfg.CanStore || cfg.CanSeal) {
				return xerrors.Errorf("must specify at least one of --store or --seal")
			}

			b, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return xerrors.Errorf("marshaling storage config: %w", err)
			}

			if err := os.WriteFile(filepath.Join(p, metaFile), b, 0644); err != nil {
				return xerrors.Errorf("persisting storage metadata (%s): %w", filepath.Join(p, metaFile), err)
			}
		}

		return minerApi.StorageAddLocal(ctx, p)
	},
}
