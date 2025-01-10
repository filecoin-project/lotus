package worker

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/docker/go-units"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const metaFile = "sectorstore.json"

var storageCmd = &cli.Command{
	Name:  "storage",
	Usage: "manage sector storage",
	Subcommands: []*cli.Command{
		storageAttachCmd,
		storageDetachCmd,
		storageRedeclareCmd,
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
		nodeApi, closer, err := lcli.GetWorkerAPI(cctx)
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

		return nodeApi.StorageAddLocal(ctx, p)
	},
}

var storageDetachCmd = &cli.Command{
	Name:  "detach",
	Usage: "detach local storage path",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "really-do-it",
		},
	},
	ArgsUsage: "[path]",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetWorkerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if !cctx.Args().Present() {
			return xerrors.Errorf("must specify storage path")
		}

		p, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("expanding path: %w", err)
		}

		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("pass --really-do-it to execute the action")
		}

		return nodeApi.StorageDetachLocal(ctx, p)
	},
}

var storageRedeclareCmd = &cli.Command{
	Name:  "redeclare",
	Usage: "redeclare sectors in a local storage path",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "id",
			Usage: "storage path ID",
		},
		&cli.BoolFlag{
			Name:  "all",
			Usage: "redeclare all storage paths",
		},
		&cli.BoolFlag{
			Name:  "drop-missing",
			Usage: "Drop index entries with missing files",
			Value: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetWorkerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		// check if no argument and no --id or --all flag is provided
		if cctx.NArg() == 0 && !cctx.IsSet("id") && !cctx.Bool("all") {
			return xerrors.Errorf("You must specify a storage path, or --id, or --all")
		}

		if cctx.IsSet("id") && cctx.Bool("all") {
			return xerrors.Errorf("--id and --all can't be passed at the same time")
		}

		if cctx.Bool("all") && cctx.NArg() > 0 {
			return xerrors.Errorf("No additional arguments are expected when --all is set")
		}

		if cctx.IsSet("id") {
			id := storiface.ID(cctx.String("id"))
			return nodeApi.StorageRedeclareLocal(ctx, &id, cctx.Bool("drop-missing"))
		}

		if cctx.Bool("all") {
			return nodeApi.StorageRedeclareLocal(ctx, nil, cctx.Bool("drop-missing"))
		}

		// As no --id or --all flag is set, we can assume the argument is a path.
		path := cctx.Args().First()
		metaFilePath := filepath.Join(path, "sectorstore.json")

		var meta storiface.LocalStorageMeta
		metaFile, err := os.Open(metaFilePath)
		if err != nil {
			return xerrors.Errorf("Failed to open file: %w", err)
		}
		defer func() {
			if closeErr := metaFile.Close(); closeErr != nil {
				log.Error("Failed to close the file: %v", closeErr)
			}
		}()

		err = json.NewDecoder(metaFile).Decode(&meta)
		if err != nil {
			return xerrors.Errorf("Failed to decode file: %w", err)
		}

		id := meta.ID
		return nodeApi.StorageRedeclareLocal(ctx, &id, cctx.Bool("drop-missing"))
	},
}
