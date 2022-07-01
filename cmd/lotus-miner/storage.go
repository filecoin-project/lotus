package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	"github.com/filecoin-project/lotus/storage/paths"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
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
		storageListCmd,
		storageFindCmd,
		storageCleanupCmd,
		storageLocks,
	},
}

var storageAttachCmd = &cli.Command{
	Name:  "attach",
	Usage: "attach local storage path",
	Description: `Storage can be attached to the miner using this command. The storage volume
list is stored local to the miner in $LOTUS_MINER_PATH/storage.json. We do not
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

			var maxStor int64
			if cctx.IsSet("max-storage") {
				maxStor, err = units.RAMInBytes(cctx.String("max-storage"))
				if err != nil {
					return xerrors.Errorf("parsing max-storage: %w", err)
				}
			}

			cfg := &paths.LocalStorageMeta{
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
		&cli.BoolFlag{
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
		},
	},
	Subcommands: []*cli.Command{
		storageListSectorsCmd,
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("color") {
			color.NoColor = !cctx.Bool("color")
		}

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
			storiface.ID
			sectors []storiface.Decl
			stat    fsutil.FsStat
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

			safeRepeat := func(s string, count int) string {
				if count < 0 {
					return ""
				}
				return strings.Repeat(s, count)
			}

			var barCols = int64(50)

			// filesystem use bar
			{
				usedPercent := (st.Capacity - st.FSAvailable) * 100 / st.Capacity

				percCol := color.FgGreen
				switch {
				case usedPercent > 98:
					percCol = color.FgRed
				case usedPercent > 90:
					percCol = color.FgYellow
				}

				set := (st.Capacity - st.FSAvailable) * barCols / st.Capacity
				used := (st.Capacity - (st.FSAvailable + st.Reserved)) * barCols / st.Capacity
				reserved := set - used
				bar := safeRepeat("#", int(used)) + safeRepeat("*", int(reserved)) + safeRepeat(" ", int(barCols-set))

				desc := ""
				if st.Max > 0 {
					desc = " (filesystem)"
				}

				fmt.Printf("\t[%s] %s/%s %s%s\n", color.New(percCol).Sprint(bar),
					types.SizeStr(types.NewInt(uint64(st.Capacity-st.FSAvailable))),
					types.SizeStr(types.NewInt(uint64(st.Capacity))),
					color.New(percCol).Sprintf("%d%%", usedPercent), desc)
			}

			// optional configured limit bar
			if st.Max > 0 {
				usedPercent := st.Used * 100 / st.Max

				percCol := color.FgGreen
				switch {
				case usedPercent > 98:
					percCol = color.FgRed
				case usedPercent > 90:
					percCol = color.FgYellow
				}

				set := st.Used * barCols / st.Max
				used := (st.Used + st.Reserved) * barCols / st.Max
				reserved := set - used
				bar := safeRepeat("#", int(used)) + safeRepeat("*", int(reserved)) + safeRepeat(" ", int(barCols-set))

				fmt.Printf("\t[%s] %s/%s %s (limit)\n", color.New(percCol).Sprint(bar),
					types.SizeStr(types.NewInt(uint64(st.Used))),
					types.SizeStr(types.NewInt(uint64(st.Max))),
					color.New(percCol).Sprintf("%d%%", usedPercent))
			}

			fmt.Printf("\t%s; %s; %s; Reserved: %s\n",
				color.YellowString("Unsealed: %d", cnt[0]),
				color.GreenString("Sealed: %d", cnt[1]),
				color.BlueString("Caches: %d", cnt[2]),
				types.SizeStr(types.NewInt(uint64(st.Reserved))))

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
			} else {
				fmt.Print(color.HiYellowString("Use: ReadOnly"))
			}
			fmt.Println()

			if len(si.Groups) > 0 {
				fmt.Printf("\tGroups: %s\n", strings.Join(si.Groups, ", "))
			}
			if len(si.AllowTo) > 0 {
				fmt.Printf("\tAllowTo: %s\n", strings.Join(si.AllowTo, ", "))
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
	id    storiface.ID
	store storiface.SectorStorageInfo

	unsealed, sealed, cache bool
	update, updatecache     bool
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
			return xerrors.New("Usage: lotus-miner storage find [sector number]")
		}

		snum, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return err
		}

		sid := abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: abi.SectorNumber(snum),
		}

		u, err := nodeApi.StorageFindSector(ctx, sid, storiface.FTUnsealed, 0, false)
		if err != nil {
			return xerrors.Errorf("finding unsealed: %w", err)
		}

		s, err := nodeApi.StorageFindSector(ctx, sid, storiface.FTSealed, 0, false)
		if err != nil {
			return xerrors.Errorf("finding sealed: %w", err)
		}

		c, err := nodeApi.StorageFindSector(ctx, sid, storiface.FTCache, 0, false)
		if err != nil {
			return xerrors.Errorf("finding cache: %w", err)
		}

		us, err := nodeApi.StorageFindSector(ctx, sid, storiface.FTUpdate, 0, false)
		if err != nil {
			return xerrors.Errorf("finding sealed: %w", err)
		}

		uc, err := nodeApi.StorageFindSector(ctx, sid, storiface.FTUpdateCache, 0, false)
		if err != nil {
			return xerrors.Errorf("finding cache: %w", err)
		}

		byId := map[storiface.ID]*storedSector{}
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
		for _, info := range us {
			sts, ok := byId[info.ID]
			if !ok {
				sts = &storedSector{
					id:    info.ID,
					store: info,
				}
				byId[info.ID] = sts
			}
			sts.update = true
		}
		for _, info := range uc {
			sts, ok := byId[info.ID]
			if !ok {
				sts = &storedSector{
					id:    info.ID,
					store: info,
				}
				byId[info.ID] = sts
			}
			sts.updatecache = true
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
			if info.update {
				types += "Update, "
			}
			if info.updatecache {
				types += "UpdateCache, "
			}

			fmt.Printf("In %s (%s)\n", info.id, types[:len(types)-2])
			fmt.Printf("\tSealing: %t; Storage: %t\n", info.store.CanSeal, info.store.CanStore)
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

var storageListSectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "get list of all sector files",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("color") {
			color.NoColor = !cctx.Bool("color")
		}

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		napi, closer2, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer2()

		ctx := lcli.ReqContext(cctx)

		sectors, err := nodeApi.SectorsList(ctx)
		if err != nil {
			return xerrors.Errorf("listing sectors: %w", err)
		}

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		aid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}

		mi, err := napi.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		sid := func(sn abi.SectorNumber) abi.SectorID {
			return abi.SectorID{
				Miner:  abi.ActorID(aid),
				Number: sn,
			}
		}

		allParts, err := getAllPartitions(ctx, maddr, napi)
		if err != nil {
			return xerrors.Errorf("getting partition states: %w", err)
		}

		type entry struct {
			id      abi.SectorNumber
			storage storiface.ID
			ft      storiface.SectorFileType
			urls    string

			primary, copy, main, seal, store bool

			state api.SectorState

			faulty bool
		}

		var list []entry

		for _, sector := range sectors {
			st, err := nodeApi.SectorsStatus(ctx, sector, false)
			if err != nil {
				return xerrors.Errorf("getting sector status for sector %d: %w", sector, err)
			}
			fault, err := allParts.FaultySectors.IsSet(uint64(sector))
			if err != nil {
				return xerrors.Errorf("checking if sector is faulty: %w", err)
			}

			for _, ft := range storiface.PathTypes {
				si, err := nodeApi.StorageFindSector(ctx, sid(sector), ft, mi.SectorSize, false)
				if err != nil {
					return xerrors.Errorf("find sector %d: %w", sector, err)
				}

				for _, info := range si {

					list = append(list, entry{
						id:      sector,
						storage: info.ID,
						ft:      ft,
						urls:    strings.Join(info.URLs, ";"),

						primary: info.Primary,
						copy:    !info.Primary && len(si) > 1,
						main:    !info.Primary && len(si) == 1, // only copy, but not primary

						seal:  info.CanSeal,
						store: info.CanStore,

						state:  st.State,
						faulty: fault,
					})
				}
			}

		}

		sort.Slice(list, func(i, j int) bool {
			if list[i].store != list[j].store {
				return list[i].store
			}

			if list[i].storage != list[j].storage {
				return list[i].storage < list[j].storage
			}

			if list[i].id != list[j].id {
				return list[i].id < list[j].id
			}

			return list[i].ft < list[j].ft
		})

		tw := tablewriter.New(
			tablewriter.Col("Storage"),
			tablewriter.Col("Sector"),
			tablewriter.Col("Type"),
			tablewriter.Col("State"),
			tablewriter.Col("Faulty"),
			tablewriter.Col("Primary"),
			tablewriter.Col("Path use"),
			tablewriter.Col("URLs"),
		)

		if len(list) == 0 {
			return nil
		}

		lastS := list[0].storage
		sc1, sc2 := color.FgBlue, color.FgCyan

		for _, e := range list {
			if e.storage != lastS {
				lastS = e.storage
				sc1, sc2 = sc2, sc1
			}

			m := map[string]interface{}{
				"Storage":  color.New(sc1).Sprint(e.storage),
				"Sector":   e.id,
				"Type":     e.ft.String(),
				"State":    color.New(stateOrder[sealing.SectorState(e.state)].col).Sprint(e.state),
				"Primary":  maybeStr(e.primary, color.FgGreen, "primary") + maybeStr(e.copy, color.FgBlue, "copy") + maybeStr(e.main, color.FgRed, "main"),
				"Path use": maybeStr(e.seal, color.FgMagenta, "seal ") + maybeStr(e.store, color.FgCyan, "store"),
				"URLs":     e.urls,
			}
			if e.faulty {
				// only set when there is a fault, so the column is hidden with no faults
				m["Faulty"] = color.RedString("faulty")
			}
			tw.Write(m)
		}

		return tw.Flush(os.Stdout)
	},
}

func getAllPartitions(ctx context.Context, maddr address.Address, napi api.FullNode) (api.Partition, error) {
	deadlines, err := napi.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return api.Partition{}, xerrors.Errorf("getting deadlines: %w", err)
	}

	out := api.Partition{
		AllSectors:        bitfield.New(),
		FaultySectors:     bitfield.New(),
		RecoveringSectors: bitfield.New(),
		LiveSectors:       bitfield.New(),
		ActiveSectors:     bitfield.New(),
	}

	for dlIdx := range deadlines {
		partitions, err := napi.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
		if err != nil {
			return api.Partition{}, xerrors.Errorf("getting partitions for deadline %d: %w", dlIdx, err)
		}

		for _, partition := range partitions {
			out.AllSectors, err = bitfield.MergeBitFields(out.AllSectors, partition.AllSectors)
			if err != nil {
				return api.Partition{}, err
			}
			out.FaultySectors, err = bitfield.MergeBitFields(out.FaultySectors, partition.FaultySectors)
			if err != nil {
				return api.Partition{}, err
			}
			out.RecoveringSectors, err = bitfield.MergeBitFields(out.RecoveringSectors, partition.RecoveringSectors)
			if err != nil {
				return api.Partition{}, err
			}
			out.LiveSectors, err = bitfield.MergeBitFields(out.LiveSectors, partition.LiveSectors)
			if err != nil {
				return api.Partition{}, err
			}
			out.ActiveSectors, err = bitfield.MergeBitFields(out.ActiveSectors, partition.ActiveSectors)
			if err != nil {
				return api.Partition{}, err
			}
		}
	}
	return out, nil
}

func maybeStr(c bool, col color.Attribute, s string) string {
	if !c {
		return ""
	}

	return color.New(col).Sprint(s)
}

var storageCleanupCmd = &cli.Command{
	Name:  "cleanup",
	Usage: "trigger cleanup actions",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "removed",
			Usage: "cleanup remaining files from removed sectors",
			Value: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		napi, closer2, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer2()

		ctx := lcli.ReqContext(cctx)

		if cctx.Bool("removed") {
			if err := cleanupRemovedSectorData(ctx, api, napi); err != nil {
				return err
			}
		}

		// TODO: proving sectors in sealing storage

		return nil
	},
}

func cleanupRemovedSectorData(ctx context.Context, api api.StorageMiner, napi v0api.FullNode) error {
	sectors, err := api.SectorsList(ctx)
	if err != nil {
		return err
	}

	maddr, err := api.ActorAddress(ctx)
	if err != nil {
		return err
	}

	aid, err := address.IDFromAddress(maddr)
	if err != nil {
		return err
	}

	sid := func(sn abi.SectorNumber) abi.SectorID {
		return abi.SectorID{
			Miner:  abi.ActorID(aid),
			Number: sn,
		}
	}

	mi, err := napi.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	toRemove := map[abi.SectorNumber]struct{}{}

	for _, sector := range sectors {
		st, err := api.SectorsStatus(ctx, sector, false)
		if err != nil {
			return xerrors.Errorf("getting sector status for sector %d: %w", sector, err)
		}

		if sealing.SectorState(st.State) != sealing.Removed {
			continue
		}

		for _, ft := range storiface.PathTypes {
			si, err := api.StorageFindSector(ctx, sid(sector), ft, mi.SectorSize, false)
			if err != nil {
				return xerrors.Errorf("find sector %d: %w", sector, err)
			}

			if len(si) > 0 {
				toRemove[sector] = struct{}{}
			}
		}
	}

	for sn := range toRemove {
		fmt.Printf("cleaning up data for sector %d\n", sn)
		err := api.SectorRemove(ctx, sn)
		if err != nil {
			log.Error(err)
		}
	}

	return nil
}

var storageLocks = &cli.Command{
	Name:  "locks",
	Usage: "show active sector locks",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		locks, err := api.StorageGetLocks(ctx)
		if err != nil {
			return err
		}

		for _, lock := range locks.Locks {
			st, err := api.SectorsStatus(ctx, lock.Sector.Number, false)
			if err != nil {
				return xerrors.Errorf("getting sector status(%d): %w", lock.Sector.Number, err)
			}

			lockstr := fmt.Sprintf("%d\t%s\t", lock.Sector.Number, color.New(stateOrder[sealing.SectorState(st.State)].col).Sprint(st.State))

			for i := 0; i < storiface.FileTypes; i++ {
				if lock.Write[i] > 0 {
					lockstr += fmt.Sprintf("%s(%s) ", storiface.SectorFileType(1<<i).String(), color.RedString("W"))
				}
				if lock.Read[i] > 0 {
					lockstr += fmt.Sprintf("%s(%s:%d) ", storiface.SectorFileType(1<<i).String(), color.GreenString("R"), lock.Read[i])
				}
			}

			fmt.Println(lockstr)
		}

		return nil
	},
}
