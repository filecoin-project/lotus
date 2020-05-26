package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/sector-storage/stores"
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
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "color"},
	},
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

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

		type fsInfo struct {
			stores.ID
			sectors []stores.Decl
			stat    stores.FsStat
		}

		sorted := make([]fsInfo, 0, len(st))
		for id, decls := range st {
			st, err := nodeApi.StorageStat(ctx, id)
			if err != nil {
				sorted = append(sorted, fsInfo{ID: id, sectors: decls})
				continue
			}

			sorted = append(sorted, fsInfo{id, decls, st})
		}

		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].stat.Capacity != sorted[j].stat.Capacity {
				return sorted[i].stat.Capacity > sorted[j].stat.Capacity
			}
			return sorted[i].ID < sorted[j].ID
		})

		for _, s := range sorted {

			var cnt [3]int
			for _, decl := range s.sectors {
				for i := range cnt {
					if decl.SectorFileType&(1<<i) != 0 {
						cnt[i]++
					}
				}
			}

			fmt.Printf("%s:\n", s.ID)

			pingStart := time.Now()
			st, err := nodeApi.StorageStat(ctx, s.ID)
			if err != nil {
				fmt.Printf("\t%s: %s:\n", color.RedString("Error"), err)
				continue
			}
			ping := time.Now().Sub(pingStart)

			usedPercent := (st.Capacity - st.Available) * 100 / st.Capacity

			percCol := color.FgGreen
			switch {
			case usedPercent > 98:
				percCol = color.FgRed
			case usedPercent > 90:
				percCol = color.FgYellow
			}

			var barCols = uint64(50)
			set := (st.Capacity - st.Available) * barCols / st.Capacity
			bar := strings.Repeat("|", int(set)) + strings.Repeat(" ", int(barCols-set))

			fmt.Printf("\t[%s] %s/%s %s\n", color.New(percCol).Sprint(bar),
				types.SizeStr(types.NewInt(st.Capacity-st.Available)),
				types.SizeStr(types.NewInt(st.Capacity)),
				color.New(percCol).Sprintf("%d%%", usedPercent))
			fmt.Printf("\t%s; %s; %s\n",
				color.YellowString("Unsealed: %d", cnt[0]),
				color.GreenString("Sealed: %d", cnt[1]),
				color.BlueString("Caches: %d", cnt[2]))

			si, err := nodeApi.StorageInfo(ctx, s.ID)
			if err != nil {
				return err
			}

			fmt.Print("\t")
			if si.CanSeal || si.CanStore {
				fmt.Printf("Weight: %d; Use: ", si.Weight)
				if si.CanSeal {
					fmt.Print(color.MagentaString("Seal "))
				}
				if si.CanStore {
					fmt.Print(color.CyanString("Store"))
				}
				fmt.Println("")
			} else {
				fmt.Print(color.HiYellowString("Use: ReadOnly"))
			}

			if localPath, ok := local[s.ID]; ok {
				fmt.Printf("\tLocal: %s\n", color.GreenString(localPath))
			}
			for i, l := range si.URLs {
				var rtt string
				if _, ok := local[s.ID]; !ok && i == 0 {
					rtt = " (latency: " + ping.Truncate(time.Microsecond*100).String() + ")"
				}

				fmt.Printf("\tURL: %s%s\n", l, rtt) // TODO; try pinging maybe?? print latency?
			}
			fmt.Println()
		}

		return nil
	},
}

type storedSector struct {
	id    stores.ID
	store stores.SectorStorageInfo

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

		u, err := nodeApi.StorageFindSector(ctx, sid, stores.FTUnsealed, false)
		if err != nil {
			return xerrors.Errorf("finding unsealed: %w", err)
		}

		s, err := nodeApi.StorageFindSector(ctx, sid, stores.FTSealed, false)
		if err != nil {
			return xerrors.Errorf("finding sealed: %w", err)
		}

		c, err := nodeApi.StorageFindSector(ctx, sid, stores.FTCache, false)
		if err != nil {
			return xerrors.Errorf("finding cache: %w", err)
		}

		byId := map[stores.ID]*storedSector{}
		for _, info := range u {
			sts, ok := byId[info.ID]
			if !ok {
				sts = &storedSector{
					id:    info.ID,
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
					id:    info.ID,
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
					id:    info.ID,
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

		var out []*storedSector
		for _, sector := range byId {
			out = append(out, sector)
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].id < out[j].id
		})

		for _, info := range out {
			var types string
			if info.unsealed {
				types += "Unsealed, "
			}
			if info.sealed {
				types += "Sealed, "
			}
			if info.cache {
				types += "Cache, "
			}

			fmt.Printf("In %s (%s)\n", info.id, types[:len(types)-2])
			fmt.Printf("\tSealing: %t; Storage: %t\n", info.store.CanSeal, info.store.CanSeal)
			if localPath, ok := local[info.id]; ok {
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
