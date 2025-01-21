package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/beacon/drand"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	proofsffi "github.com/filecoin-project/lotus/chain/proofs/ffi"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

const MAINNET_GENESIS_TIME = 1598306400

// USAGE: Sync a node, then call migrate-nv17 on some old state. Pass in the cid of the migrated state root,
// the epoch you migrated at, the network version you migrated to, and a message CID. You will be able to replay any
// message from between the migration epoch, and where your node originally synced to. Note: You may run into issues
// with state that changed between the epoch you migrated at, and when the message was originally processed.
// This can be avoided by replaying messages from close to the migration epoch, or circumvented by using a custom
// FVM bundle.
var gasTraceCmd = &cli.Command{
	Name:        "trace-gas",
	Description: "replay a message on the specified stateRoot and network version to get an execution trace",
	ArgsUsage:   "[migratedStateRootCid networkVersion messageCid]",
	Flags:       []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		if cctx.NArg() != 3 {
			return lcli.IncorrectNumArgs(cctx)
		}

		stateRootCid, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("failed to parse input: %w", err)
		}

		nv, err := strconv.ParseInt(cctx.Args().Get(1), 10, 32)
		if err != nil {
			return xerrors.Errorf("failed to parse input: %w", err)
		}

		messageCid, err := cid.Decode(cctx.Args().Get(2))
		if err != nil {
			return xerrors.Errorf("failed to parse input: %w", err)
		}

		client, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("connect to full node: %w", err)
		}
		defer closer()

		bs := blockstore.NewAPIBlockstore(client)
		mds := datastore.NewMapDatastore()

		genesis, err := client.ChainGetGenesis(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get genesis: %w", err)
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		// setup our local chainstore so it has enough data to execute a tipset, since we're not using
		// a live repo datastore
		gen := genesis.Blocks()[0]
		if err := mds.Put(ctx, datastore.NewKey("0"), gen.Cid().Bytes()); err != nil {
			return xerrors.Errorf("failed to put genesis: %w", err)
		}

		head, err := client.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get chain head: %w", err)
		}
		if err := cs.ForceHeadSilent(ctx, head); err != nil {
			return xerrors.Errorf("failed to set head: %w", err)
		}
		if err = cs.Load(ctx); err != nil {
			return xerrors.Errorf("failed to load chainstore: %w", err)
		}

		shd, err := drand.BeaconScheduleFromDrandSchedule(buildconstants.DrandConfigSchedule(), genesis.Blocks()[0].Timestamp, nil)
		if err != nil {
			return xerrors.Errorf("failed to create beacon schedule: %w", err)
		}

		sm, err := stmgr.NewStateManager(
			cs,
			consensus.NewTipSetExecutor(filcns.RewardFunc),
			vm.Syscalls(proofsffi.ProofVerifier),
			filcns.DefaultUpgradeSchedule(),
			shd,
			mds,
			nil,
		)
		if err != nil {
			return xerrors.Errorf("failed to create state manager: %w", err)
		}

		msg, err := cs.GetMessage(ctx, messageCid)
		if err != nil {
			return xerrors.Errorf("failed to get message: %w", err)
		}

		// Set to block limit so message will not run out of gas
		msg.GasLimit = buildconstants.BlockGasLimit

		res, err := sm.CallAtStateAndVersion(ctx, msg, stateRootCid, network.Version(nv))
		if err != nil {
			return xerrors.Errorf("failed to call with gas: %w", err)
		}
		_, _ = fmt.Fprintf(cctx.App.Writer, "Result: %s (%s)\n", res.MsgRct.ExitCode, res.Error)

		return printGasStats(cctx.App.Writer, res.ExecutionTrace)
	},
}

var replayOfflineCmd = &cli.Command{
	Name:        "replay-offline",
	Description: "replay a message to get a gas trace",
	ArgsUsage:   "[messageCid]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.Int64Flag{
			Name:  "lookback-limit",
			Value: 10000,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.TODO()

		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		messageCid, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("failed to parse input: %w", err)
		}

		lookbackLimit := cctx.Int("lookback-limit")

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return err
		}

		defer lkrepo.Close() //nolint:errcheck

		bs, err := lkrepo.Blockstore(ctx, repo.UniversalBlockstore)
		if err != nil {
			return xerrors.Errorf("failed to open blockstore: %w", err)
		}

		defer func() {
			if c, ok := bs.(io.Closer); ok {
				if err := c.Close(); err != nil {
					log.Warnf("failed to close blockstore: %s", err)
				}
			}
		}()

		mds, err := lkrepo.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		shd, err := drand.BeaconScheduleFromDrandSchedule(buildconstants.DrandConfigSchedule(), MAINNET_GENESIS_TIME, nil)
		if err != nil {
			return err
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(proofsffi.ProofVerifier), filcns.DefaultUpgradeSchedule(),
			shd, mds, nil)
		if err != nil {
			return err
		}

		msg, err := cs.GetMessage(ctx, messageCid)
		if err != nil {
			return err
		}

		if err = cs.Load(ctx); err != nil {
			return err
		}

		ts, _, _, err := sm.SearchForMessage(ctx, cs.GetHeaviestTipSet(), messageCid, abi.ChainEpoch(lookbackLimit), true)
		if err != nil {
			return err
		}
		if ts == nil {
			return xerrors.Errorf("could not find message within the last %d epochs", lookbackLimit)
		}
		executionTs, err := cs.GetTipsetByHeight(ctx, ts.Height()-2, ts, true)
		if err != nil {
			return err
		}

		res, err := sm.CallWithGas(ctx, msg, []types.ChainMsg{}, executionTs, true)
		if err != nil {
			return err
		}
		return printGasStats(cctx.App.Writer, res.ExecutionTrace)
	},
}

var gasSimCmd = &cli.Command{
	Name:        "gas-sim",
	Description: "simulate some variations that could affect gas usage",
	Subcommands: []*cli.Command{compareCompactSectorsCmd},
}

var compareCompactSectorsCmd = &cli.Command{
	Name:        "compact-sectors",
	Description: "replay a miner message to compare gas stats with the same message preceded by CompactPartitions",
	ArgsUsage:   "[minerMessageCid]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "lookback-limit",
			Value: "10000",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		messageCid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("failed to parse input: %w", err)
		}

		client, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("connect to full node: %w", err)
		}
		defer closer()

		bs := blockstore.NewAPIBlockstore(client)
		mds := datastore.NewMapDatastore()

		genesis, err := client.ChainGetGenesis(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get genesis: %w", err)
		}

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		// setup our local chainstore so it has enough data to execute a tipset, since we're not using
		// a live repo datastore
		gen := genesis.Blocks()[0]
		if err := mds.Put(ctx, datastore.NewKey("0"), gen.Cid().Bytes()); err != nil {
			return xerrors.Errorf("failed to put genesis: %w", err)
		}

		head, err := client.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get chain head: %w", err)
		}
		if err := cs.ForceHeadSilent(ctx, head); err != nil {
			return xerrors.Errorf("failed to set head: %w", err)
		}
		if err = cs.Load(ctx); err != nil {
			return xerrors.Errorf("failed to load chainstore: %w", err)
		}

		shd, err := drand.BeaconScheduleFromDrandSchedule(buildconstants.DrandConfigSchedule(), genesis.Blocks()[0].Timestamp, nil)
		if err != nil {
			return xerrors.Errorf("failed to create beacon schedule: %w", err)
		}

		sm, err := stmgr.NewStateManager(
			cs,
			consensus.NewTipSetExecutor(filcns.RewardFunc),
			vm.Syscalls(proofsffi.ProofVerifier),
			filcns.DefaultUpgradeSchedule(),
			shd,
			mds,
			nil,
		)
		if err != nil {
			return xerrors.Errorf("failed to create state manager: %w", err)
		}

		lookbackLimit := cctx.Int("lookback-limit")
		ts, _, mcid, err := sm.SearchForMessage(ctx, head, messageCid, abi.ChainEpoch(lookbackLimit), true)
		if err != nil {
			return xerrors.Errorf("failed to search for message: %w", err)
		}
		cmsg, err := cs.GetCMessage(ctx, mcid)
		if err != nil {
			return xerrors.Errorf("failed to get message (%s): %w", mcid, err)
		}
		msg := cmsg.VMMessage()
		pts, err := cs.LoadTipSet(ctx, ts.Parents())
		if err != nil {
			return xerrors.Errorf("failed to load parent tipset: %w", err)
		}

		stTree, err := state.LoadStateTree(cbor.NewCborStore(bs), pts.ParentState())
		if err != nil {
			return xerrors.Errorf("failed to load state tree: %w", err)
		}
		toActor, err := stTree.GetActor(msg.To)
		if err != nil {
			return xerrors.Errorf("failed to get from actor: %w", err)
		}
		if name, _, ok := actors.GetActorMetaByCode(toActor.Code); !ok {
			return xerrors.Errorf("failed to get actor meta: %w", err)
		} else if name != "storageminer" {
			return xerrors.Errorf("message is not a miner message (%s)", name)
		}

		fromActor, err := stTree.GetActor(msg.From)
		if err != nil {
			return xerrors.Errorf("failed to get from actor: %w", err)
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "Message from %s to %s:%d at height %d\n", msg.From, msg.To, msg.Method, pts.Height())

		priorMsgs := make([]types.ChainMsg, 0)

		tsMsgs, err := cs.MessagesForTipset(ctx, pts)
		if err != nil {
			return xerrors.Errorf("failed to get messages for tipset: %w", err)
		}
		for _, tsMsg := range tsMsgs {
			if tsMsg.VMMessage().From == msg.VMMessage().From && tsMsg.VMMessage().Cid() != msg.VMMessage().Cid() {
				priorMsgs = append(priorMsgs, tsMsg)
				_, _ = fmt.Fprintf(cctx.App.Writer, "Found previous message to replay (%s) from %s to %s:%d\n", tsMsg.VMMessage().Cid(), tsMsg.VMMessage().From, tsMsg.VMMessage().To, tsMsg.VMMessage().Method)
			}
		}

		// Set to block limit so message will not run out of gas
		msg.GasLimit = buildconstants.BlockGasLimit

		_, _ = fmt.Fprintf(cctx.App.Writer, "\nReplaying %d other messages prior to miner message\n", len(priorMsgs))
		origRes, err := sm.ApplyOnStateWithGas(ctx, pts.ParentState(), msg, priorMsgs, pts)
		if err != nil {
			return xerrors.Errorf("failed to call with gas: %w", err)
		}
		_, _ = fmt.Fprintf(cctx.App.Writer, "Result (%s):\n", origRes.MsgRct.ExitCode)

		if err := printTotalGasChargesPerCall(cctx.App.Writer, origRes.ExecutionTrace); err != nil {
			return xerrors.Errorf("failed to print gas stats: %w", err)
		}

		// 48 CompactPartition messages, many will fail due to partition modification rules
		nonce := fromActor.Nonce
		for i := 0; i < 48; i++ {
			partitions := bitfield.New()
			partitions.Set(0) // optimistically only doing the first partition, there may be more for this deadline, however
			cpparams := &miner.CompactPartitionsParams{
				Deadline:   uint64(i),
				Partitions: partitions,
			}
			cp, err := actors.SerializeParams(cpparams)
			if err != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}
			priorMsgs = append(priorMsgs, &types.Message{
				Version:  0,
				Value:    abi.NewTokenAmount(0),
				To:       msg.To,
				From:     msg.From,
				Method:   19, // CompactPartitions
				Params:   cp,
				GasLimit: math.MaxInt64 / 2,
				Nonce:    nonce,
			})
			nonce++
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "\nReplaying %d CompactPartitions (and other) messages prior to miner message\n", len(priorMsgs))
		cmpRes, err := sm.ApplyOnStateWithGas(ctx, pts.ParentState(), msg, priorMsgs, pts)
		if err != nil {
			return xerrors.Errorf("failed to call with gas: %w", err)
		}
		_, _ = fmt.Fprintf(cctx.App.Writer, "Result after CompactSectors (%s):\n", cmpRes.MsgRct.ExitCode)

		if err := printTotalGasChargesPerCall(cctx.App.Writer, cmpRes.ExecutionTrace); err != nil {
			return xerrors.Errorf("failed to print gas stats: %w", err)
		}

		fmt.Println("\n" + strings.Repeat("─", 120))

		origTotals := &gasTally{}
		origCharges := make(map[string]*gasTally)
		accumGasTallies(origCharges, origTotals, origRes.ExecutionTrace, false)

		cmpTotals := &gasTally{}
		cmpCharges := make(map[string]*gasTally)
		accumGasTallies(cmpCharges, cmpTotals, cmpRes.ExecutionTrace, false)

		diffCharges := make(map[string]*gasTally) // only those that differ
		for k, v := range origCharges {
			if c, ok := cmpCharges[k]; ok {
				if !v.Equal(*c) {
					diffCharges[k] = &gasTally{
						totalGas:   v.totalGas - cmpCharges[k].totalGas,
						storageGas: v.storageGas - cmpCharges[k].storageGas,
						computeGas: v.computeGas - cmpCharges[k].computeGas,
						count:      v.count - cmpCharges[k].count,
					}
				}
			} else {
				diffCharges[k] = v
			}
		}
		diffTotals := &gasTally{
			totalGas:   origTotals.totalGas - cmpTotals.totalGas,
			storageGas: origTotals.storageGas - cmpTotals.storageGas,
			computeGas: origTotals.computeGas - cmpTotals.computeGas,
			count:      origTotals.count - cmpTotals.count,
		}

		_, _ = fmt.Fprintln(cctx.App.Writer)
		_, _ = fmt.Fprintln(cctx.App.Writer, color.New(color.Bold).Sprint("Difference in gas charges for miner msg (without subcalls):"))
		if err := statsTableForCharges(cctx.App.Writer, diffCharges, diffTotals); err != nil {
			return xerrors.Errorf("failed to print gas stats: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "\n"+color.New(color.Bold).Sprint("CompactPartitions prior to message resulted in:"))
		_, _ = fmt.Fprintf(cctx.App.Writer, " • %s total gas charges\n", posNegStr(diffTotals.count, "fewer", "more"))
		_, _ = fmt.Fprintf(cctx.App.Writer, " • %s compute gas (%.2f%%)\n", posNegStr(int(diffTotals.computeGas), "less", "more"), (1-(float64(cmpTotals.computeGas)/float64(origTotals.computeGas)))*100)
		_, _ = fmt.Fprintf(cctx.App.Writer, " • %s storage gas (%.2f%%)\n", posNegStr(int(diffTotals.storageGas), "less", "more"), (1-(float64(cmpTotals.storageGas)/float64(origTotals.storageGas)))*100)
		_, _ = fmt.Fprintf(cctx.App.Writer, " • %s total gas (%.2f%%)\n", posNegStr(int(diffTotals.totalGas), "less", "more"), (1-(float64(cmpTotals.totalGas)/float64(origTotals.totalGas)))*100)
		var reads int
		if v, ok := diffCharges["OnBlockOpen"]; ok {
			reads = v.count
		}
		var writes int
		if v, ok := diffCharges["OnBlockLink"]; ok {
			writes = v.count
		}
		_, _ = fmt.Fprintf(cctx.App.Writer, " • %s reads and %s writes\n", posNegStr(reads, "fewer", "more"), posNegStr(writes, "fewer", "more"))

		return nil
	},
}

func posNegStr(i int, pos, neg string) string {
	if i >= 0 {
		return fmt.Sprintf("%d %s", i, pos)
	}
	return fmt.Sprintf("%d %s", -i, neg)
}
