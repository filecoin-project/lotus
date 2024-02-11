package main

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/rpc"
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
		storageDetachCmd,
		storageListCmd,
		/*storageDetachCmd,
		storageRedeclareCmd,
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

		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("pass --really-do-it to execute the action")
		}

		return minerApi.StorageDetachLocal(ctx, p)
	},
}

var storageListCmd = &cli.Command{
	Name:        "list",
	Usage:       "list local storage paths",
	Subcommands: []*cli.Command{
		//storageListSectorsCmd,
	},
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := rpc.GetProviderAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		st, err := minerApi.StorageList(ctx)
		if err != nil {
			return err
		}

		local, err := minerApi.StorageLocal(ctx)
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
			st, err := minerApi.StorageStat(ctx, id)
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

			var cnt [5]int
			for _, decl := range s.sectors {
				for i := range cnt {
					if decl.SectorFileType&(1<<i) != 0 {
						cnt[i]++
					}
				}
			}

			fmt.Printf("%s:\n", s.ID)

			pingStart := time.Now()
			st, err := minerApi.StorageStat(ctx, s.ID)
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

			fmt.Printf("\t%s; %s; %s; %s; %s; Reserved: %s\n",
				color.YellowString("Unsealed: %d", cnt[0]),
				color.GreenString("Sealed: %d", cnt[1]),
				color.BlueString("Caches: %d", cnt[2]),
				color.GreenString("Updated: %d", cnt[3]),
				color.BlueString("Update-caches: %d", cnt[4]),
				types.SizeStr(types.NewInt(uint64(st.Reserved))))

			si, err := minerApi.StorageInfo(ctx, s.ID)
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

			if len(si.AllowTypes) > 0 || len(si.DenyTypes) > 0 {
				denied := storiface.FTAll.SubAllowed(si.AllowTypes, si.DenyTypes)
				allowed := storiface.FTAll ^ denied

				switch {
				case bits.OnesCount64(uint64(allowed)) == 0:
					fmt.Printf("\tAllow Types: %s\n", color.RedString("None"))
				case bits.OnesCount64(uint64(allowed)) < bits.OnesCount64(uint64(denied)):
					fmt.Printf("\tAllow Types: %s\n", color.GreenString(strings.Join(allowed.Strings(), " ")))
				default:
					fmt.Printf("\tDeny Types:  %s\n", color.RedString(strings.Join(denied.Strings(), " ")))
				}
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
