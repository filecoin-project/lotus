package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	//"github.com/filecoin-project/specs-actors/actors/builtin/power"
	//"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/tools/stats"
)

var log = logging.Logger("main")

func main() {
	local := []*cli.Command{
		runCmd,
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
		Version: build.UserVersion(),
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
			Name:    "percent-extra",
			EnvVars: []string{"LOTUS_PCR_PERCENT_EXTRA"},
			Usage:   "extra funds to send above the refund",
			Value:   3,
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
		&cli.IntFlag{
			Name:    "head-delay",
			EnvVars: []string{"LOTUS_PCR_HEAD_DELAY"},
			Usage:   "the number of tipsets to delay message processing to smooth chain reorgs",
			Value:   int(build.MessageConfidence),
		},
		&cli.IntFlag{
			Name:    "miner-total-funds-threashold",
			EnvVars: []string{"LOTUS_PCR_MINER_TOTAL_FUNDS_THREASHOLD"},
			Usage:   "total filecoin across all accounts that should be met, if the miner balancer drops below zero",
			Value:   0,
		},
	},
	Action: func(cctx *cli.Context) error {
		go func() {
			http.ListenAndServe(":6060", nil) //nolint:errcheck
		}()

		ctx := context.Background()
		api, closer, err := stats.GetFullNodeAPI(cctx.Context, cctx.String("lotus-path"))
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
			if err := stats.WaitForSyncComplete(ctx, api); err != nil {
				log.Fatal(err)
			}
		}

		tipsetsCh, err := stats.GetTips(ctx, api, r.Height(), cctx.Int("head-delay"))
		if err != nil {
			log.Fatal(err)
		}

		percentExtra := cctx.Int("percent-extra")
		maxMessageQueue := cctx.Int("max-message-queue")
		dryRun := cctx.Bool("dry-run")
		preCommitEnabled := cctx.Bool("pre-commit")
		proveCommitEnabled := cctx.Bool("prove-commit")
		aggregateTipsets := cctx.Int("aggregate-tipsets")
		minerTotalFundsThreashold := uint64(cctx.Int("miner-total-funds-threashold"))

		rf := &refunder{
			api:                       api,
			wallet:                    from,
			percentExtra:              percentExtra,
			dryRun:                    dryRun,
			preCommitEnabled:          preCommitEnabled,
			proveCommitEnabled:        proveCommitEnabled,
			minerTotalFundsThreashold: types.FromFil(minerTotalFundsThreashold),
		}

		var refunds *MinersRefund = NewMinersRefund()
		var rounds int = 0

		for tipset := range tipsetsCh {
			refunds, err = rf.ProcessTipset(ctx, tipset, refunds)
			if err != nil {
				return err
			}

			rounds = rounds + 1
			if rounds < aggregateTipsets {
				continue
			}

			refundTipset, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			{
				negativeBalancerRefund, err := rf.EnsureMinerMinimums(ctx, refundTipset, NewMinersRefund())
				if err != nil {
					return err
				}

				if err := rf.Refund(ctx, "refund negative balancer miner", refundTipset, negativeBalancerRefund, rounds); err != nil {
					return err
				}
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
					time.Sleep(time.Duration(int64(time.Second) * int64(build.BlockDelaySecs)))
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
				time.Sleep(time.Duration(int64(time.Second) * int64(build.BlockDelaySecs)))
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
	StateMinerInitialPledgeCollateral(ctx context.Context, addr address.Address, precommitInfo miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateSectorPreCommitInfo(ctx context.Context, addr address.Address, sector abi.SectorNumber, tsk types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)
	StateListMiners(context.Context, types.TipSetKey) ([]address.Address, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error)
	GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error)
	WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error)
}

type refunder struct {
	api                       refunderNodeApi
	wallet                    address.Address
	percentExtra              int
	dryRun                    bool
	preCommitEnabled          bool
	proveCommitEnabled        bool
	minerTotalFundsThreashold big.Int
}

func (r *refunder) EnsureMinerMinimums(ctx context.Context, tipset *types.TipSet, refunds *MinersRefund) (*MinersRefund, error) {
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

		if minerAvailableBalance.GreaterThanEqual(big.Zero()) {
			log.Debugw("skipping over miner with positive balance ", "height", tipset.Height(), "key", tipset.Key(), "miner", maddr)
			continue
		}

		// Look up and find all addresses associated with the miner
		minerInfo, err := r.api.StateMinerInfo(ctx, maddr, tipset.Key())

		allAddresses := []address.Address{minerInfo.Worker}
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

		totalAvailableBalance := big.Add(addrSum, minerAvailableBalance)

		// If the miner has available balance they should use it
		if totalAvailableBalance.GreaterThanEqual(r.minerTotalFundsThreashold) {
			log.Debugw("skipping over miner with enough funds cross all accounts", "height", tipset.Height(), "key", tipset.Key(), "miner", maddr, "miner_available_balance", minerAvailableBalance, "wallet_total_balances", addrSum)
			continue
		}

		// Calculate the required FIL to bring the miner up to the minimum across all of their accounts
		refundValue := big.Add(totalAvailableBalance.Abs(), r.minerTotalFundsThreashold)
		refunds.Track(maddr, refundValue)

		log.Debugw("processing negative balance miner", "miner", maddr, "refund", refundValue)
	}

	return refunds, nil
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

	refundValue := types.NewInt(0)
	tipsetRefunds := NewMinersRefund()
	for i, msg := range msgs {
		m := msg.Message

		a, err := r.api.StateGetActor(ctx, m.To, tipset.Key())
		if err != nil {
			log.Warnw("failed to look up state actor", "height", tipset.Height(), "key", tipset.Key(), "actor", m.To)
			continue
		}

		if a.Code != builtin.StorageMinerActorCodeID {
			continue
		}

		var messageMethod string

		switch m.Method {
		case builtin.MethodsMiner.ProveCommitSector:
			if !r.proveCommitEnabled {
				continue
			}

			messageMethod = "ProveCommitSector"

			if recps[i].ExitCode != exitcode.Ok {
				log.Debugw("skipping non-ok exitcode message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "exitcode", recps[i].ExitCode)
				continue
			}

			var proveCommitSector miner.ProveCommitSectorParams
			if err := proveCommitSector.UnmarshalCBOR(bytes.NewBuffer(m.Params)); err != nil {
				log.Warnw("failed to decode provecommit params", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To)
				continue
			}

			// We use the parent tipset key because precommit information is removed when ProveCommitSector is executed
			precommitChainInfo, err := r.api.StateSectorPreCommitInfo(ctx, m.To, proveCommitSector.SectorNumber, tipset.Parents())
			if err != nil {
				log.Warnw("failed to get precommit info for sector", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", proveCommitSector.SectorNumber)
				continue
			}

			precommitTipset, err := r.api.ChainGetTipSetByHeight(ctx, precommitChainInfo.PreCommitEpoch, tipset.Key())
			if err != nil {
				log.Warnf("failed to lookup precommit epoch", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", proveCommitSector.SectorNumber)
				continue
			}

			collateral, err := r.api.StateMinerInitialPledgeCollateral(ctx, m.To, precommitChainInfo.Info, precommitTipset.Key())
			if err != nil {
				log.Warnw("failed to get initial pledge collateral", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", proveCommitSector.SectorNumber)
			}

			collateral = big.Sub(collateral, precommitChainInfo.PreCommitDeposit)
			if collateral.LessThan(big.Zero()) {
				log.Debugw("skipping zero pledge collateral difference", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", proveCommitSector.SectorNumber)
				continue
			}

			refundValue = collateral
		case builtin.MethodsMiner.PreCommitSector:
			if !r.preCommitEnabled {
				continue
			}

			messageMethod = "PreCommitSector"

			if recps[i].ExitCode != exitcode.Ok {
				log.Debugw("skipping non-ok exitcode message", "method", messageMethod, "cid", msg.Cid, "miner", m.To, "exitcode", recps[i].ExitCode)
				continue
			}

			var precommitInfo miner.SectorPreCommitInfo
			if err := precommitInfo.UnmarshalCBOR(bytes.NewBuffer(m.Params)); err != nil {
				log.Warnw("failed to decode precommit params", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To)
				continue
			}

			collateral, err := r.api.StateMinerInitialPledgeCollateral(ctx, m.To, precommitInfo, tipset.Key())
			if err != nil {
				log.Warnw("failed to calculate initial pledge collateral", "err", err, "method", messageMethod, "cid", msg.Cid, "miner", m.To, "sector_number", precommitInfo.SectorNumber)
				continue
			}

			refundValue = collateral
		default:
			continue
		}

		if r.percentExtra > 0 {
			refundValue = types.BigAdd(refundValue, types.BigMul(types.BigDiv(refundValue, types.NewInt(100)), types.NewInt(uint64(r.percentExtra))))
		}

		log.Debugw("processing message", "method", messageMethod, "cid", msg.Cid, "from", m.From, "to", m.To, "value", m.Value, "gas_fee_cap", m.GasFeeCap, "gas_premium", m.GasPremium, "gas_used", recps[i].GasUsed, "refund", refundValue)

		refunds.Track(m.From, refundValue)
		tipsetRefunds.Track(m.From, refundValue)
	}

	log.Infow("tipset stats", "height", tipset.Height(), "key", tipset.Key(), "total_refunds", tipsetRefunds.TotalRefunds(), "messages_processed", tipsetRefunds.Count())

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

	log.Infow(name, "tipsets_processed", rounds, "height", tipset.Height(), "key", tipset.Key(), "messages_sent", len(messages)-failures, "refund_sum", refundSum, "messages_failures", failures, "messages_processed", refunds.Count())
	return nil
}

type Repo struct {
	last abi.ChainEpoch
	path string
}

func NewRepo(path string) (*Repo, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, err
	}

	return &Repo{
		last: 0,
		path: path,
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

func (r *Repo) Open() (err error) {
	if err = r.init(); err != nil {
		return
	}

	var f *os.File

	f, err = os.OpenFile(filepath.Join(r.path, "height"), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer func() {
		err = f.Close()
	}()

	var raw []byte

	raw, err = ioutil.ReadAll(f)
	if err != nil {
		return
	}

	height, err := strconv.Atoi(string(bytes.TrimSpace(raw)))
	if err != nil {
		return
	}

	r.last = abi.ChainEpoch(height)
	return
}

func (r *Repo) Height() abi.ChainEpoch {
	return r.last
}

func (r *Repo) SetHeight(last abi.ChainEpoch) (err error) {
	r.last = last
	var f *os.File
	f, err = os.OpenFile(filepath.Join(r.path, "height"), os.O_RDWR, 0644)
	if err != nil {
		return
	}

	defer func() {
		err = f.Close()
	}()

	if _, err = fmt.Fprintf(f, "%d", r.last); err != nil {
		return
	}

	return
}
