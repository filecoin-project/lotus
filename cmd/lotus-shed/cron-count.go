package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ipfs/go-cid"
	ipldcbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	miner11 "github.com/filecoin-project/go-state-types/builtin/v11/miner"
	"github.com/filecoin-project/go-state-types/builtin/v11/util/adt"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var cronWcCmd = &cli.Command{
	Name:        "cron-wc",
	Description: "cron stats",
	Subcommands: []*cli.Command{
		minerDeadlineCronCountCmd,
		minerDeadlinePartitionMeasurementCmd,
	},
}

type DeadlineRef struct {
	To     string
	Height abi.ChainEpoch
	Gas    json.RawMessage
}

type DeadlineSummary struct {
	Partitions      []PartitionSummary
	PreCommitExpiry PreCommitExpiry
	VestingDiff     VestingDiff
}

type PreCommitExpiry struct {
	Expired []uint64
}

type VestingDiff struct {
	PrevTableSize int
	NewTableSize  int
}

type PartitionSummary struct {
	Live   int
	Dead   int
	Faulty int
	Diff   PartitionDiff
}

type PartitionDiff struct {
	Faulted   int
	Recovered int
	Killed    int
}

var minerDeadlinePartitionMeasurementCmd = &cli.Command{
	Name:        "deadline-summary",
	Description: "",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "json",
			Usage: "read input as json",
			Value: true,
		},
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset state to search on (pass comma separated array of cids)",
		},
	},
	Action: func(c *cli.Context) error {
		// read in values to process
		if !c.Bool("json") {
			return xerrors.Errorf("unsupported non json input format")
		}
		var refStream []DeadlineRef
		if err := json.NewDecoder(os.Stdin).Decode(&refStream); err != nil {
			return xerrors.Errorf("failed to parse input: %w", err)
		}

		// go from height and sp addr to deadline partition data
		n, acloser, err := lcli.GetFullNodeAPI(c)
		if err != nil {
			return err
		}
		defer acloser()
		ctx := lcli.ReqContext(c)

		bs := ReadOnlyAPIBlockstore{n}
		adtStore := adt.WrapStore(ctx, ipldcbor.NewCborStore(&bs))

		dSummaries := make([]DeadlineSummary, len(refStream))
		for j, ref := range refStream {
			// get miner's deadline
			tsBefore, err := n.ChainGetTipSetByHeight(ctx, ref.Height, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("failed to get tipset at epoch: %d: %w", ref.Height, err)
			}
			tsAfter, err := n.ChainGetTipSetByHeight(ctx, ref.Height+1, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("failed to get tipset at epoch %d: %w", ref.Height, err)
			}
			addr, err := address.NewFromString(ref.To)
			if err != nil {
				return xerrors.Errorf("faield to get address from input string: %w", err)
			}
			dline, err := n.StateMinerProvingDeadline(ctx, addr, tsBefore.Key())
			if err != nil {
				return xerrors.Errorf("failed to read proving deadline: %w", err)
			}

			// iterate through all partitions at epoch of processing
			var pSummaries []PartitionSummary
			psBefore, err := n.StateMinerPartitions(ctx, addr, dline.Index, tsBefore.Key())
			if err != nil {
				return xerrors.Errorf("failed to get partitions: %w", err)
			}
			psAfter, err := n.StateMinerPartitions(ctx, addr, dline.Index, tsAfter.Key())
			if err != nil {
				return xerrors.Errorf("failed to get partitions: %w", err)
			}
			if len(psBefore) != len(psAfter) {
				return xerrors.Errorf("faield")
			}

			type partitionCount struct {
				live       int
				dead       int
				faulty     int
				recovering int
			}
			countPartition := func(p api.Partition) (partitionCount, error) {
				liveSectors, err := p.LiveSectors.All(abi.MaxSectorNumber)
				if err != nil {
					return partitionCount{}, xerrors.Errorf("failed to count live sectors in partition: %w", err)
				}
				allSectors, err := p.AllSectors.All(abi.MaxSectorNumber)
				if err != nil {
					return partitionCount{}, xerrors.Errorf("failed to count all sectors in partition: %w", err)
				}
				faultySectors, err := p.FaultySectors.All(abi.MaxSectorNumber)
				if err != nil {
					return partitionCount{}, xerrors.Errorf("failed to count faulty sectors in partition: %w", err)
				}
				recoveringSectors, err := p.RecoveringSectors.All(abi.MaxSectorNumber)
				if err != nil {
					return partitionCount{}, xerrors.Errorf("failed to count recovering sectors in partition: %w", err)
				}

				return partitionCount{
					live:       len(liveSectors),
					dead:       len(allSectors) - len(liveSectors),
					faulty:     len(faultySectors),
					recovering: len(recoveringSectors),
				}, nil
			}

			countVestingTable := func(table cid.Cid) (int, error) {
				var vestingTable miner11.VestingFunds
				if err := adtStore.Get(ctx, table, &vestingTable); err != nil {
					return 0, err
				}
				return len(vestingTable.Funds), nil
			}

			for i := 0; i < len(psBefore); i++ {
				cntBefore, err := countPartition(psBefore[i])
				if err != nil {
					return err
				}
				cntAfter, err := countPartition(psAfter[i])
				if err != nil {
					return err
				}
				pSummaries = append(pSummaries, PartitionSummary{
					Live:   cntBefore.live,
					Dead:   cntBefore.dead,
					Faulty: cntBefore.faulty,
					Diff: PartitionDiff{
						Faulted:   cntAfter.faulty - cntBefore.faulty,
						Recovered: cntBefore.recovering - cntAfter.recovering,
						Killed:    cntAfter.dead - cntBefore.dead,
					},
				})
			}

			// Precommit and vesting table data
			// Before
			aBefore, err := n.StateGetActor(ctx, addr, tsBefore.Key())
			if err != nil {
				return err
			}
			var st miner11.State
			err = adtStore.Get(ctx, aBefore.Head, &st)
			if err != nil {
				return err
			}
			expiryQArray, err := adt.AsArray(adtStore, st.PreCommittedSectorsCleanUp, miner11.PrecommitCleanUpAmtBitwidth)
			if err != nil {
				return err
			}
			var sectorsBf bitfield.BitField
			var accumulator []uint64
			h := ref.Height
			if err := expiryQArray.ForEach(&sectorsBf, func(i int64) error {
				if abi.ChainEpoch(i) > h {
					return nil
				}
				sns, err := sectorsBf.All(abi.MaxSectorNumber)
				if err != nil {
					return err
				}
				accumulator = append(accumulator, sns...)
				return nil
			}); err != nil {
				return err
			}

			vestingBefore, err := countVestingTable(st.VestingFunds)
			if err != nil {
				return err
			}

			// After
			aAfter, err := n.StateGetActor(ctx, addr, tsAfter.Key())
			if err != nil {
				return err
			}
			var stAfter miner11.State
			err = adtStore.Get(ctx, aAfter.Head, &stAfter)
			if err != nil {
				return err
			}

			vestingAfter, err := countVestingTable(stAfter.VestingFunds)
			if err != nil {
				return err
			}

			dSummaries[j] = DeadlineSummary{
				Partitions: pSummaries,
				PreCommitExpiry: PreCommitExpiry{
					Expired: accumulator,
				},
				VestingDiff: VestingDiff{
					PrevTableSize: vestingBefore,
					NewTableSize:  vestingAfter,
				},
			}

		}

		// output partition info
		if err := json.NewEncoder(os.Stdout).Encode(dSummaries); err != nil {
			return err
		}
		return nil
	},
}

var minerDeadlineCronCountCmd = &cli.Command{
	Name:        "deadline",
	Description: "list all addresses of miners with active deadline crons",
	Action: func(c *cli.Context) error {
		return countDeadlineCrons(c)
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset state to search on (pass comma separated array of cids)",
		},
	},
}

func findDeadlineCrons(c *cli.Context) (map[address.Address]struct{}, error) {
	api, acloser, err := lcli.GetFullNodeAPI(c)
	if err != nil {
		return nil, err
	}
	defer acloser()
	ctx := lcli.ReqContext(c)

	ts, err := lcli.LoadTipSet(ctx, c, api)
	if err != nil {
		return nil, err
	}
	if ts == nil {
		ts, err = api.ChainHead(ctx)
		if err != nil {
			return nil, err
		}
	}

	mAddrs, err := api.StateListMiners(ctx, ts.Key())
	if err != nil {
		return nil, err
	}
	activeMiners := make(map[address.Address]struct{})
	for _, mAddr := range mAddrs {
		// All miners have active cron before v4.
		// v4 upgrade epoch is last epoch running v3 epoch and api.StateReadState reads
		// parent state, so v4 state isn't read until upgrade epoch + 2
		if ts.Height() <= build.UpgradeTurboHeight+1 {
			activeMiners[mAddr] = struct{}{}
			continue
		}
		st, err := api.StateReadState(ctx, mAddr, ts.Key())
		if err != nil {
			return nil, err
		}
		minerState, ok := st.State.(map[string]interface{})
		if !ok {
			return nil, xerrors.Errorf("internal error: failed to cast miner state to expected map type")
		}

		activeDlineIface, ok := minerState["DeadlineCronActive"]
		if !ok {
			return nil, xerrors.Errorf("miner %s had no deadline state, is this a v3 state root?", mAddr)
		}
		active := activeDlineIface.(bool)
		if active {
			activeMiners[mAddr] = struct{}{}
		}
	}

	return activeMiners, nil
}

func countDeadlineCrons(c *cli.Context) error {
	activeMiners, err := findDeadlineCrons(c)
	if err != nil {
		return err
	}
	for addr := range activeMiners {
		fmt.Printf("%s\n", addr)
	}

	return nil
}
