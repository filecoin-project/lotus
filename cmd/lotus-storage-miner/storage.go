package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"

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
		storageFindCmd,
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

			cfg := &stores.LocalStorageMeta{
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
	Name:  "list",
	Usage: "list local storage paths",
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

		local, err := nodeApi.StorageLocal(ctx)
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
			fmt.Printf("\tSeal: %t; Store: %t; Weight: %d\n", si.CanSeal, si.CanStore, si.Weight)
			if localPath, ok := local[id]; ok {
				fmt.Printf("\tLocal: %s\n", localPath)
			}
			for _, l := range si.URLs {
				fmt.Printf("\tURL: %s\n", l) // TODO; try pinging maybe?? print latency?
			}
		}

		return nil
	},
}

type storedSector struct {
	store                   stores.StorageInfo
	unsealed, sealed, cache bool
}

var storageFindCmd = &cli.Command{
	Name:      "find",
	Usage:     "find sector in the storage system",
	ArgsUsage: "[sector number]",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		ma, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mid, err := address.IDFromAddress(ma)
		if err != nil {
			return err
		}

		if !cctx.Args().Present() {
			return xerrors.New("Usage: lotus-storage-miner storage find [sector number]")
		}

		snum, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return err
		}

		sid := abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: abi.SectorNumber(snum),
		}

		u, err := nodeApi.StorageFindSector(ctx, sid, sectorbuilder.FTUnsealed)
		if err != nil {
			return xerrors.Errorf("finding unsealed: %w", err)
		}

		s, err := nodeApi.StorageFindSector(ctx, sid, sectorbuilder.FTSealed)
		if err != nil {
			return xerrors.Errorf("finding sealed: %w", err)
		}

		c, err := nodeApi.StorageFindSector(ctx, sid, sectorbuilder.FTCache)
		if err != nil {
			return xerrors.Errorf("finding cache: %w", err)
		}

		byId := map[stores.ID]*storedSector{}
		for _, info := range u {
			sts, ok := byId[info.ID]
			if !ok {
				sts = &storedSector{
					store: info,
				}
				byId[info.ID] = sts
			}
			sts.unsealed = true
		}
		for _, info := range s {
			sts, ok := byId[info.ID]
			if !ok {
				sts = &storedSector{
					store: info,
				}
				byId[info.ID] = sts
			}
			sts.sealed = true
		}
		for _, info := range c {
			sts, ok := byId[info.ID]
			if !ok {
				sts = &storedSector{
					store: info,
				}
				byId[info.ID] = sts
			}
			sts.cache = true
		}

		local, err := nodeApi.StorageLocal(ctx)
		if err != nil {
			return err
		}

		for id, info := range byId {
			fmt.Printf("In %s (Unsealed: %t; Sealed: %t; Cache: %t)\n", id, info.unsealed, info.sealed, info.cache)
			fmt.Printf("\tSealing: %t; Storage: %t\n", info.store.CanSeal, info.store.CanSeal)
			if localPath, ok := local[id]; ok {
				fmt.Printf("\tLocal (%s)\n", localPath)
			} else {
				fmt.Printf("\tRemote\n")
			}
			for _, l := range info.store.URLs {
				fmt.Printf("\tURL: %s\n", l)
			}
		}

		return nil
	},
}
