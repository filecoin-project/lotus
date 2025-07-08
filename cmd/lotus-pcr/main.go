package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	lbuiltin "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/tools/stats/sync"
)

var log = logging.Logger("main")

func main() {
	local := []*cli.Command{
		runCmd,
		recoverMinersCmd,
		findMinersCmd,
		versionCmd,
	}

	app := &cli.App{
		Name:  "lotus-pcr",
		Usage: "Refunds precommit initial pledge for all miners",
		Description: `Lotus PCR will attempt to reimbursement the initial pledge collateral of the PreCommitSector
   miner actor method for all miners on the network.

   The refund is sent directly to the miner actor, and not to the worker.

   The value refunded to the miner actor is not the value in the message itself, but calculated
   using StateMinerInitialPledgeCollateral of the PreCommitSector message params. This is to reduce
   abuse by over send in the PreCommitSector message and receiving more funds than was actually
   consumed by pledging the sector.

   No gas charges are refunded as part of this process, but a small 3% (by default) additional
   funds are provided.

   A single message will be produced per miner totaling their refund for all PreCommitSector messages
   in a tipset.
`,
		Version: string(build.NodeUserVersion()),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "lotus-path",
				EnvVars: []string{"LOTUS_PATH"},
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PCR_PATH"},
				Value:   "~/.lotuspcr", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "log-level",
				EnvVars: []string{"LOTUS_PCR_LOG_LEVEL"},
				Hidden:  true,
				Value:   "info",
			},
		},
		Before: func(cctx *cli.Context) error {
			return logging.SetLogLevel("main", cctx.String("log-level"))
		},
		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorw("exit in error", "err", err)
		os.Exit(1)
		return
	}
}

var versionCmd = &cli.Command{
	Name:  "version",
	Usage: "Print version",
	Action: func(cctx *cli.Context) error {
		cli.VersionPrinter(cctx)
		return nil
	},
}

var findMinersCmd = &cli.Command{
	Name:  "find-miners",
	Usage: "find miners with a desired minimum balance",
	Description: `Find miners returns a list of miners and their balances that are below a
   threshold value. By default only the miner actor available balance is considered but other
   account balances can be included by enabling them through the flags.

   Examples
   Find all miners with an available balance below 100 FIL

     lotus-pcr find-miners --threshold 100

   Find all miners with a balance below zero, which includes the owner and worker balances

     lotus-pcr find-miners --threshold 0 --owner --worker
`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "no-sync",
			EnvVars: []string{"LOTUS_PCR_NO_SYNC"},
			Usage:   "do not wait for chain sync to complete",
		},
		&cli.IntFlag{
			Name:    "threshold",
			EnvVars: []string{"LOTUS_PCR_THRESHOLD"},
			Usage:   "balance below this limit will be printed",
			Value:   0,
		},
		&cli.BoolFlag{
			Name:  "owner",
			Usage: "include owner balance",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "worker",
			Usage: "include worker balance",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "control",
			Usage: "include control balance",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		if !cctx.Bool("no-sync") {
			if err := sync.SyncWait(ctx, api); err != nil {
				return err
			}
		}

		owner := cctx.Bool("owner")
		worker := cctx.Bool("worker")
		control := cctx.Bool("control")
		threshold := uint64(cctx.Int("threshold"))

		rf := &refunder{
			api:       api,
			threshold: types.FromFil(threshold),
		}

		refundTipset, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		balanceRefund, err := rf.FindMiners(ctx, refundTipset, NewMinersRefund(), owner, worker, control)
		if err != nil {
			return err
		}

		for _, maddr := range balanceRefund.Miners() {
			fmt.Printf("%s\t%s\n", maddr, types.FIL(balanceRefund.GetRefund(maddr)))
		}

		return nil
	},
}

var recoverMinersCmd = &cli.Command{
	Name:  "recover-miners",
	Usage: "Ensure all miners with a negative available balance have a FIL surplus across accounts",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "from",
			EnvVars: []string{"LOTUS_PCR_FROM"},
			Usage:   "wallet address to send refund from",
		},
		&cli.BoolFlag{
			Name:    "no-sync",
			EnvVars: []string{"LOTUS_PCR_NO_SYNC"},
			Usage:   "do not wait for chain sync to complete",
		},
		&cli.BoolFlag{
			Name:    "dry-run",
			EnvVars: []string{"LOTUS_PCR_DRY_RUN"},
			Usage:   "do not send any messages",
			Value:   false,
		},
		&cli.StringFlag{
			Name:  "output",
			Usage: "dump data as a csv format to this file",
		},
		&cli.IntFlag{
			Name:    "miner-recovery-cutoff",
			EnvVars: []string{"LOTUS_PCR_MINER_RECOVERY_CUTOFF"},
			Usage:   "maximum amount of FIL that can be sent to any one miner before refund percent is applied",
			Value:   3000,
		},
		&cli.IntFlag{
			Name:    "miner-recovery-bonus",
			EnvVars: []string{"LOTUS_PCR_MINER_RECOVERY_BONUS"},
			Usage:   "additional FIL to send to each miner",
			Value:   5,
		},
		&cli.IntFlag{
			Name:    "miner-recovery-refund-percent",
			EnvVars: []string{"LOTUS_PCR_MINER_RECOVERY_REFUND_PERCENT"},
			Usage:   "percent of refund to issue",
			Value:   110,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			log.Fatal(err)
		}
		defer closer()

		r, err := NewRepo(cctx.String("repo"))
		if err != nil {
			return err
		}

		if err := r.Open(); err != nil {
			return err
		}

		from, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return xerrors.Errorf("parsing source address (provide correct --from flag!): %w", err)
		}

		if !cctx.Bool("no-sync") {
			if err := sync.SyncWait(ctx, api); err != nil {
				return err
			}
		}

		dryRun := cctx.Bool("dry-run")
		minerRecoveryRefundPercent := cctx.Int("miner-recovery-refund-percent")
		minerRecoveryCutoff := uint64(cctx.Int("miner-recovery-cutoff"))
		minerRecoveryBonus := uint64(cctx.Int("miner-recovery-bonus"))

		blockmap := make(map[address.Address]struct{})

		for _, addr := range r.Blocklist() {
			blockmap[addr] = struct{}{}
		}

		rf := &refunder{
			api:                        api,
			wallet:                     from,
			dryRun:                     dryRun,
			minerRecoveryRefundPercent: minerRecoveryRefundPercent,
			minerRecoveryCutoff:        types.FromFil(minerRecoveryCutoff),
			minerRecoveryBonus:         types.FromFil(minerRecoveryBonus),
			blockmap:                   blockmap,
		}

		refundTipset, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		balanceRefund, err := rf.EnsureMinerMinimums(ctx, refundTipset, NewMinersRefund(), cctx.String("output"))
		if err != nil {
			return err
		}

		if err := rf.Refund(ctx, "refund to recover miner", refundTipset, balanceRefund, 0); err != nil {
			return err
		}

		return nil
	},
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start message reimpursement",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "from",
			EnvVars: []string{"LOTUS_PCR_FROM"},
			Usage:   "wallet address to send refund from",
		},
		&cli.BoolFlag{
			Name:    "no-sync",
			EnvVars: []string{"LOTUS_PCR_NO_SYNC"},
			Usage:   "do not wait for chain sync to complete",
		},
		&cli.IntFlag{
			Name:    "refund-percent",
			EnvVars: []string{"LOTUS_PCR_REFUND_PERCENT"},
			Usage:   "percent of refund to issue",
			Value:   103,
		},
		&cli.IntFlag{
			Name:    "max-message-queue",
			EnvVars: []string{"LOTUS_PCR_MAX_MESSAGE_QUEUE"},
			Usage:   "set the maximum number of messages that can be queue in the mpool",
			Value:   300,
		},
		&cli.IntFlag{
			Name:    "aggregate-tipsets",
			EnvVars: []string{"LOTUS_PCR_AGGREGATE_TIPSETS"},
			Usage:   "number of tipsets to process before sending messages",
			Value:   1,
		},
		&cli.BoolFlag{
			Name:    "dry-run",
			EnvVars: []string{"LOTUS_PCR_DRY_RUN"},
			Usage:   "do not send any messages",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:    "pre-commit",
			EnvVars: []string{"LOTUS_PCR_PRE_COMMIT"},
			Usage:   "process PreCommitSector messages",
			Value:   true,
		},
		&cli.BoolFlag{
			Name:    "prove-commit",
			EnvVars: []string{"LOTUS_PCR_PROVE_COMMIT"},
			Usage:   "process ProveCommitSector messages",
			Value:   true,
		},
		&cli.BoolFlag{
			Name:    "windowed-post",
			EnvVars: []string{"LOTUS_PCR_WINDOWED_POST"},
			Usage:   "process SubmitWindowedPoSt messages and refund gas fees",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:    "storage-deals",
			EnvVars: []string{"LOTUS_PCR_STORAGE_DEALS"},
			Usage:   "process PublishStorageDeals messages and refund gas fees",
			Value:   false,
		},
		&cli.IntFlag{
			Name:    "head-delay",
			EnvVars: []string{"LOTUS_PCR_HEAD_DELAY"},
			Usage:   "the number of tipsets to delay message processing to smooth chain reorgs",
			Value:   int(buildconstants.MessageConfidence),
		},
		&cli.BoolFlag{
			Name:    "miner-recovery",
			EnvVars: []string{"LOTUS_PCR_MINER_RECOVERY"},
			Usage:   "run the miner recovery job",
			Value:   false,
		},
		&cli.IntFlag{
			Name:    "miner-recovery-period",
			EnvVars: []string{"LOTUS_PCR_MINER_RECOVERY_PERIOD"},
			Usage:   "interval between running miner recovery",
			Value:   2880,
		},
		&cli.IntFlag{
			Name:    "miner-recovery-cutoff",
			EnvVars: []string{"LOTUS_PCR_MINER_RECOVERY_CUTOFF"},
			Usage:   "maximum amount of FIL that can be sent to any one miner before refund percent is applied",
			Value:   3000,
		},
		&cli.IntFlag{
			Name:    "miner-recovery-bonus",
			EnvVars: []string{"LOTUS_PCR_MINER_RECOVERY_BONUS"},
			Usage:   "additional FIL to send to each miner",
			Value:   5,
		},
		&cli.IntFlag{
			Name:    "miner-recovery-refund-percent",
			EnvVars: []string{"LOTUS_PCR_MINER_RECOVERY_REFUND_PERCENT"},
			Usage:   "percent of refund to issue",
			Value:   110,
		},
		&cli.StringFlag{
			Name:    "pre-fee-cap-max",
			EnvVars: []string{"LOTUS_PCR_PRE_FEE_CAP_MAX"},
			Usage:   "messages with a fee cap larger than this will be skipped when processing pre commit messages",
			Value:   "0.000000001",
		},
		&cli.StringFlag{
			Name:    "prove-fee-cap-max",
			EnvVars: []string{"LOTUS_PCR_PROVE_FEE_CAP_MAX"},
			Usage:   "messages with a prove cap larger than this will be skipped when processing pre commit messages",
			Value:   "0.000000001",
		},
		&cli.StringFlag{
			Name:  "http-server-timeout",
			Value: "30s",
		},
	},
	Action: func(cctx *cli.Context) error {
		timeout, err := time.ParseDuration(cctx.String("http-timeout"))
		if err != nil {
			return xerrors.Errorf("invalid time string %s: %x", cctx.String("http-timeout"), err)
		}

		go func() {
			server := &http.Server{
				Addr:              ":6060",
				ReadHeaderTimeout: timeout,
			}

			_ = server.ListenAndServe()
		}()

		ctx := context.Background()
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			log.Fatal(err)
		}
		defer closer()

		r, err := NewRepo(cctx.String("repo"))
		if err != nil {
			return err
		}

		if err := r.Open(); err != nil {
			return err
		}

		from, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return xerrors.Errorf("parsing source address (provide correct --from flag!): %w", err)
		}

		if !cctx.Bool("no-sync") {
			if err := sync.SyncWait(ctx, api); err != nil {
				return err
			}
		}

		tipsetsCh, err := sync.BufferedTipsetChannel(ctx, api, r.Height(), cctx.Int("head-delay"))
		if err != nil {
			log.Fatal(err)
		}

		refundPercent := cctx.Int("refund-percent")
		maxMessageQueue := cctx.Int("max-message-queue")
		dryRun := cctx.Bool("dry-run")
		preCommitEnabled := cctx.Bool("pre-commit")
		proveCommitEnabled := cctx.Bool("prove-commit")
		windowedPoStEnabled := cctx.Bool("windowed-post")
		publishStorageDealsEnabled := cctx.Bool("storage-deals")
		aggregateTipsets := cctx.Int("aggregate-tipsets")
		minerRecoveryEnabled := cctx.Bool("miner-recovery")
		minerRecoveryPeriod := abi.ChainEpoch(int64(cctx.Int("miner-recovery-period")))
		minerRecoveryRefundPercent := cctx.Int("miner-recovery-refund-percent")
		minerRecoveryCutoff := uint64(cctx.Int("miner-recovery-cutoff"))
		minerRecoveryBonus := uint64(cctx.Int("miner-recovery-bonus"))

		preFeeCapMax, err := types.ParseFIL(cctx.String("pre-fee-cap-max"))
		if err != nil {
			return err
		}

		proveFeeCapMax, err := types.ParseFIL(cctx.String("prove-fee-cap-max"))
		if err != nil {
			return err
		}

		blockmap := make(map[address.Address]struct{})

		for _, addr := range r.Blocklist() {
			blockmap[addr] = struct{}{}
		}

		rf := &refunder{
			api:                        api,
			wallet:                     from,
			refundPercent:              refundPercent,
			minerRecoveryRefundPercent: minerRecoveryRefundPercent,
			minerRecoveryCutoff:        types.FromFil(minerRecoveryCutoff),
			minerRecoveryBonus:         types.FromFil(minerRecoveryBonus),
			dryRun:                     dryRun,
			preCommitEnabled:           preCommitEnabled,
			proveCommitEnabled:         proveCommitEnabled,
			windowedPoStEnabled:        windowedPoStEnabled,
			publishStorageDealsEnabled: publishStorageDealsEnabled,
			preFeeCapMax:               types.BigInt(preFeeCapMax),
			proveFeeCapMax:             types.BigInt(proveFeeCapMax),
			blockmap:                   blockmap,
		}

		var refunds = NewMinersRefund()
		var rounds = 0
		nextMinerRecovery := r.MinerRecoveryHeight() + minerRecoveryPeriod

		for tipset := range tipsetsCh {
			for k := range rf.blockmap {
				fmt.Printf("%s\n", k)
			}

			refunds, err = rf.ProcessTipset(ctx, tipset, refunds)
			if err != nil {
				return err
			}

			refundTipset, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			if minerRecoveryEnabled && refundTipset.Height() >= nextMinerRecovery {
				recoveryRefund, err := rf.EnsureMinerMinimums(ctx, refundTipset, NewMinersRefund(), "")
				if err != nil {
					return err
				}

				if err := rf.Refund(ctx, "refund to recover miners", refundTipset, recoveryRefund, 0); err != nil {
					return err
				}

				if err := r.SetMinerRecoveryHeight(tipset.Height()); err != nil {
					return err
				}

				nextMinerRecovery = r.MinerRecoveryHeight() + minerRecoveryPeriod
			}

			rounds = rounds + 1
			if rounds < aggregateTipsets {
				continue
			}

			if err := rf.Refund(ctx, "refund stats", refundTipset, refunds, rounds); err != nil {
				return err
			}

			rounds = 0
			refunds = NewMinersRefund()

			if err := r.SetHeight(tipset.Height()); err != nil {
				return err
			}

			for {
				msgs, err := api.MpoolPending(ctx, types.EmptyTSK)
				if err != nil {
					log.Warnw("failed to fetch pending messages", "err", err)
					time.Sleep(time.Duration(int64(time.Second) * int64(buildconstants.BlockDelaySecs)))
					continue
				}

				count := 0
				for _, msg := range msgs {
					if msg.Message.From == from {
						count = count + 1
					}
				}

				if count < maxMessageQueue {
					break
				}

				log.Warnw("messages in mpool over max message queue", "message_count", count, "max_message_queue", maxMessageQueue)
				time.Sleep(time.Duration(int64(time.Second) * int64(buildconstants.BlockDelaySecs)))
			}
		}

		return nil
	},
}

type MinersRefund struct {
	refunds      map[address.Address]types.BigInt
	count        int
	totalRefunds types.BigInt
}

func NewMinersRefund() *MinersRefund {
	return &MinersRefund{
		refunds:      make(map[address.Address]types.BigInt),
		totalRefunds: types.NewInt(0),
	}
}

func (m *MinersRefund) Track(addr address.Address, value types.BigInt) {
	if _, ok := m.refunds[addr]; !ok {
		m.refunds[addr] = types.NewInt(0)
	}

	m.count = m.count + 1
	m.totalRefunds = types.BigAdd(m.totalRefunds, value)
	m.refunds[addr] = types.BigAdd(m.refunds[addr], value)
}

func (m *MinersRefund) Count() int {
	return m.count
}

func (m *MinersRefund) TotalRefunds() types.BigInt {
	return m.totalRefunds
}

func (m *MinersRefund) Miners() []address.Address {
	miners := make([]address.Address, 0, len(m.refunds))
	for addr := range m.refunds {
		miners = append(miners, addr)
	}

	return miners
}

func (m *MinersRefund) GetRefund(addr address.Address) types.BigInt {
	return m.refunds[addr]
}

type refunderNodeApi interface {
	ChainGetParentMessages(ctx context.Context, blockCid cid.Cid) ([]api.Message, error)
	ChainGetParentReceipts(ctx context.Context, blockCid cid.Cid) ([]*types.MessageReceipt, error)
	ChainGetTipSetByHeight(ctx context.Context, epoch abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	StateMinerInitialPledgeCollateral(ctx context.Context, addr address.Address, precommitInfo minertypes.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateSectorPreCommitInfo(ctx context.Context, addr address.Address, sector abi.SectorNumber, tsk types.TipSetKey) (minertypes.SectorPreCommitOnChainInfo, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)
	StateMinerSectors(ctx context.Context, addr address.Address, filter *bitfield.BitField, tsk types.TipSetKey) ([]*miner.SectorOnChainInfo, error)
	StateMinerFaults(ctx context.Context, addr address.Address, tsk types.TipSetKey) (bitfield.BitField, error)
	StateListMiners(context.Context, types.TipSetKey) ([]address.Address, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error)
	MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error)
	GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error)
	WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error)
}

type refunder struct {
	api                        refunderNodeApi
	wallet                     address.Address
	refundPercent              int
	minerRecoveryRefundPercent int
	minerRecoveryCutoff        big.Int
	minerRecoveryBonus         big.Int
	dryRun                     bool
	preCommitEnabled           bool
	proveCommitEnabled         bool
	windowedPoStEnabled        bool
	publishStorageDealsEnabled bool
	threshold                  big.Int
	blockmap                   map[address.Address]struct{}

	preFeeCapMax   big.Int
	proveFeeCapMax big.Int
}

func (r *refunder) FindMiners(ctx context.Context, tipset *types.TipSet, refunds *MinersRefund, owner, worker, control bool) (*MinersRefund, error) {
	miners, err := r.api.StateListMiners(ctx, tipset.Key())
	if err != nil {
		return nil, err
	}

	for _, maddr := range miners {
		mact, err := r.api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			log.Errorw("failed", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		if !mact.Balance.GreaterThan(big.Zero()) {
			continue
		}

		minerAvailableBalance, err := r.api.StateMinerAvailableBalance(ctx, maddr, tipset.Key())
		if err != nil {
			log.Errorw("failed", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		// Look up and find all addresses associated with the miner
		minerInfo, err := r.api.StateMinerInfo(ctx, maddr, tipset.Key())
		if err != nil {
			log.Errorw("failed", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		allAddresses := []address.Address{}

		if worker {
			allAddresses = append(allAddresses, minerInfo.Worker)
		}

		if owner {
			allAddresses = append(allAddresses, minerInfo.Owner)
		}

		if control {
			allAddresses = append(allAddresses, minerInfo.ControlAddresses...)
		}

		// Sum the balancer of all the addresses
		addrSum := big.Zero()
		addrCheck := make(map[address.Address]struct{}, len(allAddresses))
		for _, addr := range allAddresses {
			if _, found := addrCheck[addr]; !found {
				balance, err := r.api.WalletBalance(ctx, addr)
				if err != nil {
					log.Errorw("failed", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
					continue
				}

				addrSum = big.Add(addrSum, balance)
				addrCheck[addr] = struct{}{}
			}
		}

		totalAvailableBalance := big.Add(addrSum, minerAvailableBalance)

		if totalAvailableBalance.GreaterThanEqual(r.threshold) {
			continue
		}

		refunds.Track(maddr, totalAvailableBalance)

		log.Debugw("processing miner", "miner", maddr, "sectors", "available_balance", totalAvailableBalance)
	}

	return refunds, nil
}

func (r *refunder) EnsureMinerMinimums(ctx context.Context, tipset *types.TipSet, refunds *MinersRefund, output string) (*MinersRefund, error) {
	miners, err := r.api.StateListMiners(ctx, tipset.Key())
	if err != nil {
		return nil, err
	}

	w := io.Discard
	if len(output) != 0 {
		f, err := os.Create(output)
		if err != nil {
			return nil, err
		}

		defer f.Close() // nolint:errcheck

		w = bufio.NewWriter(f)
	}

	csvOut := csv.NewWriter(w)
	defer csvOut.Flush()
	if err := csvOut.Write([]string{"MinerID", "FaultedSectors", "AvailableBalance", "ProposedRefund"}); err != nil {
		return nil, err
	}

	for _, maddr := range miners {
		if _, found := r.blockmap[maddr]; found {
			log.Debugw("skipping blocked miner", "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		mact, err := r.api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			log.Errorw("failed", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		if !mact.Balance.GreaterThan(big.Zero()) {
			continue
		}

		minerAvailableBalance, err := r.api.StateMinerAvailableBalance(ctx, maddr, tipset.Key())
		if err != nil {
			log.Errorw("failed", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		// Look up and find all addresses associated with the miner
		minerInfo, err := r.api.StateMinerInfo(ctx, maddr, tipset.Key())
		if err != nil {
			log.Errorw("failed", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		allAddresses := []address.Address{minerInfo.Worker, minerInfo.Owner}
		allAddresses = append(allAddresses, minerInfo.ControlAddresses...)

		// Sum the balancer of all the addresses
		addrSum := big.Zero()
		addrCheck := make(map[address.Address]struct{}, len(allAddresses))
		for _, addr := range allAddresses {
			if _, found := addrCheck[addr]; !found {
				balance, err := r.api.WalletBalance(ctx, addr)
				if err != nil {
					log.Errorw("failed", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
					continue
				}

				addrSum = big.Add(addrSum, balance)
				addrCheck[addr] = struct{}{}
			}
		}

		faults, err := r.api.StateMinerFaults(ctx, maddr, tipset.Key())
		if err != nil {
			log.Errorw("failed to look up miner faults", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		faultsCount, err := faults.Count()
		if err != nil {
			log.Errorw("failed to get count of faults", "err", err, "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		if faultsCount == 0 {
			log.Debugw("skipping miner with zero faults", "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		totalAvailableBalance := big.Add(addrSum, minerAvailableBalance)
		balanceCutoff := big.Mul(big.Div(big.NewIntUnsigned(faultsCount), big.NewInt(10)), big.NewIntUnsigned(buildconstants.FilecoinPrecision))

		if totalAvailableBalance.GreaterThan(balanceCutoff) {
			log.Debugw(
				"skipping over miner with total available balance larger than refund",
				"height", tipset.Height(),
				"key", tipset.Key(),
				"miner", maddr,
				"available_balance", totalAvailableBalance,
				"balance_cutoff", balanceCutoff,
				"faults_count", faultsCount,
				"available_balance_fil", big.Div(totalAvailableBalance, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
				"balance_cutoff_fil", big.Div(balanceCutoff, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
			)
			continue
		}

		refundValue := big.Sub(balanceCutoff, totalAvailableBalance)
		if r.minerRecoveryRefundPercent > 0 {
			refundValue = types.BigMul(types.BigDiv(refundValue, types.NewInt(100)), types.NewInt(uint64(r.minerRecoveryRefundPercent)))
		}

		refundValue = big.Add(refundValue, r.minerRecoveryBonus)

		if refundValue.GreaterThan(r.minerRecoveryCutoff) {
			log.Infow(
				"skipping over miner with refund greater than refund cutoff",
				"height", tipset.Height(),
				"key", tipset.Key(),
				"miner", maddr,
				"available_balance", totalAvailableBalance,
				"balance_cutoff", balanceCutoff,
				"faults_count", faultsCount,
				"refund", refundValue,
				"available_balance_fil", big.Div(totalAvailableBalance, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
				"balance_cutoff_fil", big.Div(balanceCutoff, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
				"refund_fil", big.Div(refundValue, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
			)
			continue
		}

		refunds.Track(maddr, refundValue)
		record := []string{
			maddr.String(),
			fmt.Sprintf("%d", faultsCount),
			big.Div(totalAvailableBalance, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).String(),
			big.Div(refundValue, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).String(),
		}
		if err := csvOut.Write(record); err != nil {
			return nil, err
		}

		log.Debugw(
			"processing miner",
			"miner", maddr,
			"faults_count", faultsCount,
			"available_balance", totalAvailableBalance,
			"refund", refundValue,
			"available_balance_fil", big.Div(totalAvailableBalance, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
			"refund_fil", big.Div(refundValue, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
		)
	}

	return refunds, nil
}

func (r *refunder) processTipsetStorageMarketActor(ctx context.Context, tipset *types.TipSet, msg api.Message, recp *types.MessageReceipt) (bool, string, types.BigInt, error) {

	m := msg.Message
	var refundValue types.BigInt
	var messageMethod string

	switch m.Method {
	case market.Methods.PublishStorageDeals:
		if !r.publishStorageDealsEnabled {
			return false, messageMethod, types.NewInt(0), nil
		}

		messageMethod = "PublishStorageDeals"

		if recp.ExitCode != exitcode.Ok {
			log.Debugw("skipping non-ok exitcode message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "exitcode", recp.ExitCode)
			return false, messageMethod, types.NewInt(0), nil
		}

		refundValue = types.BigMul(types.NewInt(uint64(recp.GasUsed)), tipset.Blocks()[0].ParentBaseFee)
	default:
		return false, messageMethod, types.NewInt(0), nil
	}

	return true, messageMethod, refundValue, nil
}

func (r *refunder) processTipsetStorageMinerActor(ctx context.Context, tipset *types.TipSet, msg api.Message, recp *types.MessageReceipt) (bool, string, types.BigInt, error) {

	m := msg.Message
	var refundValue types.BigInt
	var messageMethod string

	if _, found := r.blockmap[m.To]; found {
		log.Debugw("skipping blocked miner", "height", tipset.Height(), "key", tipset.Key(), "miner", m.To)
		return false, messageMethod, types.NewInt(0), nil
	}

	switch m.Method {
	case builtin.MethodsMiner.SubmitWindowedPoSt:
		if !r.windowedPoStEnabled {
			return false, messageMethod, types.NewInt(0), nil
		}

		messageMethod = "SubmitWindowedPoSt"

		if recp.ExitCode != exitcode.Ok {
			log.Debugw("skipping non-ok exitcode message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "exitcode", recp.ExitCode)
			return false, messageMethod, types.NewInt(0), nil
		}

		refundValue = types.BigMul(types.NewInt(uint64(recp.GasUsed)), tipset.Blocks()[0].ParentBaseFee)
	case builtin.MethodsMiner.ProveCommitSector:
		if !r.proveCommitEnabled {
			return false, messageMethod, types.NewInt(0), nil
		}

		messageMethod = "ProveCommitSector"

		if recp.ExitCode != exitcode.Ok {
			log.Debugw("skipping non-ok exitcode message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "exitcode", recp.ExitCode)
			return false, messageMethod, types.NewInt(0), nil
		}

		if m.GasFeeCap.GreaterThan(r.proveFeeCapMax) {
			log.Debugw("skipping high fee cap message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "gas_fee_cap", m.GasFeeCap, "fee_cap_max", r.proveFeeCapMax)
			return false, messageMethod, types.NewInt(0), nil
		}

		if tipset.Blocks()[0].ParentBaseFee.GreaterThan(r.proveFeeCapMax) {
			log.Debugw("skipping high base fee message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "basefee", tipset.Blocks()[0].ParentBaseFee, "fee_cap_max", r.proveFeeCapMax)
			return false, messageMethod, types.NewInt(0), nil
		}

		var sn abi.SectorNumber

		var proveCommitSector miner2.ProveCommitSectorParams
		if err := proveCommitSector.UnmarshalCBOR(bytes.NewBuffer(m.Params)); err != nil {
			log.Warnw("failed to decode provecommit params", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To)
			return false, messageMethod, types.NewInt(0), nil
		}

		sn = proveCommitSector.SectorNumber

		// We use the parent tipset key because precommit information is removed when ProveCommitSector is executed
		precommitChainInfo, err := r.api.StateSectorPreCommitInfo(ctx, m.To, sn, tipset.Parents())
		if err != nil {
			log.Warnw("failed to get precommit info for sector", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", sn)
			return false, messageMethod, types.NewInt(0), nil
		}

		precommitTipset, err := r.api.ChainGetTipSetByHeight(ctx, precommitChainInfo.PreCommitEpoch, tipset.Key())
		if err != nil {
			log.Warnf("failed to lookup precommit epoch", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", sn)
			return false, messageMethod, types.NewInt(0), nil
		}

		collateral, err := r.api.StateMinerInitialPledgeCollateral(ctx, m.To, precommitChainInfo.Info, precommitTipset.Key())
		if err != nil {
			log.Warnw("failed to get initial pledge collateral", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", sn)
			return false, messageMethod, types.NewInt(0), nil
		}

		collateral = big.Sub(collateral, precommitChainInfo.PreCommitDeposit)
		if collateral.LessThan(big.Zero()) {
			log.Debugw("skipping zero pledge collateral difference", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", sn)
			return false, messageMethod, types.NewInt(0), nil
		}

		refundValue = collateral
		if r.refundPercent > 0 {
			refundValue = types.BigMul(types.BigDiv(refundValue, types.NewInt(100)), types.NewInt(uint64(r.refundPercent)))
		}
	case builtin.MethodsMiner.PreCommitSector:
		if !r.preCommitEnabled {
			return false, messageMethod, types.NewInt(0), nil
		}

		messageMethod = "PreCommitSector"

		if recp.ExitCode != exitcode.Ok {
			log.Debugw("skipping non-ok exitcode message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "exitcode", recp.ExitCode)
			return false, messageMethod, types.NewInt(0), nil
		}

		if m.GasFeeCap.GreaterThan(r.preFeeCapMax) {
			log.Debugw("skipping high fee cap message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "gas_fee_cap", m.GasFeeCap, "fee_cap_max", r.preFeeCapMax)
			return false, messageMethod, types.NewInt(0), nil
		}

		if tipset.Blocks()[0].ParentBaseFee.GreaterThan(r.preFeeCapMax) {
			log.Debugw("skipping high base fee message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "basefee", tipset.Blocks()[0].ParentBaseFee, "fee_cap_max", r.preFeeCapMax)
			return false, messageMethod, types.NewInt(0), nil
		}

		var precommitInfo minertypes.SectorPreCommitInfo
		if err := precommitInfo.UnmarshalCBOR(bytes.NewBuffer(m.Params)); err != nil {
			log.Warnw("failed to decode precommit params", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To)
			return false, messageMethod, types.NewInt(0), nil
		}

		collateral, err := r.api.StateMinerInitialPledgeCollateral(ctx, m.To, precommitInfo, tipset.Key())
		if err != nil {
			log.Warnw("failed to calculate initial pledge collateral", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", precommitInfo.SectorNumber)
			return false, messageMethod, types.NewInt(0), nil
		}

		refundValue = collateral
		if r.refundPercent > 0 {
			refundValue = types.BigMul(types.BigDiv(refundValue, types.NewInt(100)), types.NewInt(uint64(r.refundPercent)))
		}
	default:
		return false, messageMethod, types.NewInt(0), nil
	}

	return true, messageMethod, refundValue, nil
}

func (r *refunder) ProcessTipset(ctx context.Context, tipset *types.TipSet, refunds *MinersRefund) (*MinersRefund, error) {
	cids := tipset.Cids()
	if len(cids) == 0 {
		log.Errorw("no cids in tipset", "height", tipset.Height(), "key", tipset.Key())
		return nil, fmt.Errorf("no cids in tipset")
	}

	msgs, err := r.api.ChainGetParentMessages(ctx, cids[0])
	if err != nil {
		log.Errorw("failed to get tipset parent messages", "err", err, "height", tipset.Height(), "key", tipset.Key())
		return nil, nil
	}

	recps, err := r.api.ChainGetParentReceipts(ctx, cids[0])
	if err != nil {
		log.Errorw("failed to get tipset parent receipts", "err", err, "height", tipset.Height(), "key", tipset.Key())
		return nil, nil
	}

	if len(msgs) != len(recps) {
		log.Errorw("message length does not match receipts length", "height", tipset.Height(), "key", tipset.Key(), "messages", len(msgs), "receipts", len(recps))
		return nil, nil
	}

	tipsetRefunds := NewMinersRefund()
	for i, msg := range msgs {
		refundValue := types.NewInt(0)
		m := msg.Message

		a, err := r.api.StateGetActor(ctx, m.To, tipset.Key())
		if err != nil {
			log.Warnw("failed to look up state actor", "height", tipset.Height(), "key", tipset.Key(), "actor", m.To)
			continue
		}

		var messageMethod string
		var processed bool

		if m.To == market.Address {
			processed, messageMethod, refundValue, err = r.processTipsetStorageMarketActor(ctx, tipset, msg, recps[i])
		}

		if lbuiltin.IsStorageMinerActor(a.Code) {
			processed, messageMethod, refundValue, err = r.processTipsetStorageMinerActor(ctx, tipset, msg, recps[i])
		}

		if err != nil {
			log.Errorw("error while processing message", "cid", msg.Cid)
			continue
		}
		if !processed {
			continue
		}

		log.Debugw(
			"processing message",
			"method", messageMethod,
			"cid", msg.Cid,
			"from", m.From,
			"to", m.To,
			"value", m.Value,
			"gas_fee_cap", m.GasFeeCap,
			"gas_premium", m.GasPremium,
			"gas_used", recps[i].GasUsed,
			"refund", refundValue,
			"refund_fil", big.Div(refundValue, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
		)

		refunds.Track(m.From, refundValue)
		tipsetRefunds.Track(m.From, refundValue)
	}

	log.Infow(
		"tipset stats",
		"height", tipset.Height(),
		"key", tipset.Key(),
		"total_refunds", tipsetRefunds.TotalRefunds(),
		"total_refunds_fil", big.Div(tipsetRefunds.TotalRefunds(), big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
		"messages_processed", tipsetRefunds.Count(),
	)

	return refunds, nil
}

func (r *refunder) Refund(ctx context.Context, name string, tipset *types.TipSet, refunds *MinersRefund, rounds int) error {
	if refunds.Count() == 0 {
		log.Debugw("no messages to refund in tipset", "height", tipset.Height(), "key", tipset.Key())
		return nil
	}

	var messages []*types.Message
	refundSum := types.NewInt(0)

	for _, maddr := range refunds.Miners() {
		refundValue := refunds.GetRefund(maddr)

		// We want to try and ensure these messages get mined quickly
		gasPremium, err := r.api.GasEstimateGasPremium(ctx, 0, r.wallet, 0, tipset.Key())
		if err != nil {
			log.Warnw("failed to estimate gas premium", "err", err, "height", tipset.Height(), "key", tipset.Key())
			continue
		}

		msg := &types.Message{
			Value: refundValue,
			From:  r.wallet,
			To:    maddr,

			GasPremium: gasPremium,
		}

		refundSum = types.BigAdd(refundSum, msg.Value)
		messages = append(messages, msg)
	}

	balance, err := r.api.WalletBalance(ctx, r.wallet)
	if err != nil {
		log.Errorw("failed to get wallet balance", "err", err, "height", tipset.Height(), "key", tipset.Key())
		return xerrors.Errorf("failed to get wallet balance :%w", err)
	}

	// Calculate the minimum balance as the total refund we need to issue plus 5% to cover fees
	minBalance := types.BigAdd(refundSum, types.BigDiv(refundSum, types.NewInt(500)))
	if balance.LessThan(minBalance) {
		log.Errorw("not sufficient funds to cover refunds", "balance", balance, "refund_sum", refundSum, "minimum_required", minBalance)
		return xerrors.Errorf("wallet does not have enough balance to cover refund")
	}

	failures := 0
	refundSum.SetUint64(0)
	for _, msg := range messages {
		if !r.dryRun {
			if _, err = r.api.MpoolPushMessage(ctx, msg, nil); err != nil {
				log.Errorw("failed to MpoolPushMessage", "err", err, "msg", msg)
				failures = failures + 1
				continue
			}
		}

		refundSum = types.BigAdd(refundSum, msg.Value)
	}

	log.Infow(
		name,
		"tipsets_processed", rounds,
		"height", tipset.Height(),
		"key", tipset.Key(),
		"messages_sent", len(messages)-failures,
		"refund_sum", refundSum,
		"refund_sum_fil", big.Div(refundSum, big.NewIntUnsigned(buildconstants.FilecoinPrecision)).Int64(),
		"messages_failures", failures,
		"messages_processed", refunds.Count(),
	)
	return nil
}

type Repo struct {
	lastHeight              abi.ChainEpoch
	lastMinerRecoveryHeight abi.ChainEpoch
	path                    string
	blocklist               []address.Address
}

func NewRepo(path string) (*Repo, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, err
	}

	return &Repo{
		lastHeight:              0,
		lastMinerRecoveryHeight: 0,
		path:                    path,
	}, nil
}

func (r *Repo) exists() (bool, error) {
	_, err := os.Stat(r.path)
	notexist := os.IsNotExist(err)
	if notexist {
		err = nil
	}
	return !notexist, err

}

func (r *Repo) init() error {
	exist, err := r.exists()
	if err != nil {
		return err
	}
	if exist {
		return nil
	}

	err = os.Mkdir(r.path, 0755) //nolint: gosec
	if err != nil && !os.IsExist(err) {
		return err
	}

	return nil
}

func (r *Repo) Open() error {
	if err := r.init(); err != nil {
		return err
	}

	if err := r.loadHeight(); err != nil {
		return err
	}

	if err := r.loadMinerRecoveryHeight(); err != nil {
		return err
	}

	if err := r.loadBlockList(); err != nil {
		return err
	}

	return nil
}

func loadChainEpoch(fn string) (abi.ChainEpoch, error) {
	f, err := os.OpenFile(fn, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer func() {
		err = f.Close()
	}()

	raw, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}

	height, err := strconv.Atoi(string(bytes.TrimSpace(raw)))
	if err != nil {
		return 0, err
	}

	return abi.ChainEpoch(height), nil
}

func (r *Repo) loadBlockList() error {
	var err error
	fpath := filepath.Join(r.path, "blocklist")
	f, err := os.OpenFile(fpath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func() {
		err = f.Close()
	}()

	blocklist := []address.Address{}
	input := bufio.NewReader(f)
	for {
		stra, errR := input.ReadString('\n')
		stra = strings.TrimSpace(stra)

		if len(stra) == 0 {
			if errR == io.EOF {
				break
			}
			continue
		}

		addr, err := address.NewFromString(stra)
		if err != nil {
			return err
		}

		blocklist = append(blocklist, addr)

		if errR != nil && errR != io.EOF {
			return err
		}

		if errR == io.EOF {
			break
		}
	}

	r.blocklist = blocklist

	return nil
}

func (r *Repo) loadHeight() error {
	var err error
	r.lastHeight, err = loadChainEpoch(filepath.Join(r.path, "height"))
	return err
}

func (r *Repo) loadMinerRecoveryHeight() error {
	var err error
	r.lastMinerRecoveryHeight, err = loadChainEpoch(filepath.Join(r.path, "miner_recovery_height"))
	return err
}

func (r *Repo) Blocklist() []address.Address {
	return r.blocklist
}

func (r *Repo) Height() abi.ChainEpoch {
	return r.lastHeight
}

func (r *Repo) MinerRecoveryHeight() abi.ChainEpoch {
	return r.lastMinerRecoveryHeight
}

func (r *Repo) SetHeight(last abi.ChainEpoch) (err error) {
	r.lastHeight = last
	var f *os.File
	f, err = os.OpenFile(filepath.Join(r.path, "height"), os.O_RDWR, 0644)
	if err != nil {
		return
	}

	defer func() {
		err = f.Close()
	}()

	if _, err = fmt.Fprintf(f, "%d", r.lastHeight); err != nil {
		return
	}

	return
}

func (r *Repo) SetMinerRecoveryHeight(last abi.ChainEpoch) (err error) {
	r.lastMinerRecoveryHeight = last
	var f *os.File
	f, err = os.OpenFile(filepath.Join(r.path, "miner_recovery_height"), os.O_RDWR, 0644)
	if err != nil {
		return
	}

	defer func() {
		err = f.Close()
	}()

	if _, err = fmt.Fprintf(f, "%d", r.lastMinerRecoveryHeight); err != nil {
		return
	}

	return
}
