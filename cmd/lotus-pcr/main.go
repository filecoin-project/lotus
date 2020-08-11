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

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/tools/stats"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
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
		},

		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		log.Warn(err)
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
		},
		&cli.BoolFlag{
			Name:    "no-sync",
			EnvVars: []string{"LOTUS_PCR_NO_SYNC"},
		},
		&cli.IntFlag{
			Name:  "percent-extra",
			Value: 3,
		},
		&cli.IntFlag{
			Name:  "head-delay",
			Usage: "the number of tipsets to delay message processing to smooth chain reorgs",
			Value: int(build.MessageConfidence),
		},
	},
	Action: func(cctx *cli.Context) error {
		go func() {
			http.ListenAndServe(":6060", nil)
		}()

		ctx := context.Background()
		api, closer, err := stats.GetFullNodeAPI(cctx.String("lotus-path"))
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

		for tipset := range tipsetsCh {
			if err := ProcessTipset(ctx, api, tipset, from, percentExtra); err != nil {
				return err
			}

			if err := r.SetHeight(tipset.Height()); err != nil {
				return err
			}
		}

		return nil
	},
}

type MinersRefund struct {
	refunds map[address.Address]types.BigInt
}

func NewMinersRefund() *MinersRefund {
	return &MinersRefund{
		refunds: make(map[address.Address]types.BigInt),
	}
}

func (m *MinersRefund) Track(addr address.Address, value types.BigInt) {
	if _, ok := m.refunds[addr]; !ok {
		m.refunds[addr] = types.NewInt(0)
	}

	m.refunds[addr] = types.BigAdd(m.refunds[addr], value)
}

func (m *MinersRefund) Count() int {
	return len(m.refunds)
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

type processTipSetApi interface {
	ChainGetParentMessages(ctx context.Context, blockCid cid.Cid) ([]api.Message, error)
	ChainGetParentReceipts(ctx context.Context, blockCid cid.Cid) ([]*types.MessageReceipt, error)
	StateMinerInitialPledgeCollateral(ctx context.Context, addr address.Address, precommitInfo miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	MpoolPushMessage(ctx context.Context, msg *types.Message) (*types.SignedMessage, error)
	GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error)
	WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error)
}

func ProcessTipset(ctx context.Context, api processTipSetApi, tipset *types.TipSet, wallet address.Address, percentExtra int) error {
	log.Infow("processing tipset", "height", tipset.Height(), "key", tipset.Key().String())

	cids := tipset.Cids()
	if len(cids) == 0 {
		return fmt.Errorf("no cids in tipset")
	}

	msgs, err := api.ChainGetParentMessages(ctx, cids[0])
	if err != nil {
		log.Errorw("failed to get tipset parent messages", "err", err)
		return nil
	}

	recps, err := api.ChainGetParentReceipts(ctx, cids[0])
	if err != nil {
		log.Errorw("failed to get tipset parent receipts", "err", err)
		return nil
	}

	if len(msgs) != len(recps) {
		log.Errorw("message length does not match receipts length", "messages", len(msgs), "receipts", len(recps))
		return nil
	}

	refunds := NewMinersRefund()

	count := 0
	for i, msg := range msgs {
		m := msg.Message

		a, err := api.StateGetActor(ctx, m.To, tipset.Key())
		if err != nil {
			log.Warnw("failed to look up state actor", "actor", m.To)
			continue
		}

		if a.Code != builtin.StorageMinerActorCodeID {
			continue
		}

		// we only care to look at PreCommitSector messages
		if m.Method != builtin.MethodsMiner.PreCommitSector {
			continue
		}

		if recps[i].ExitCode != exitcode.Ok {
			log.Debugw("skipping non-ok exitcode message", "cid", msg.Cid.String(), "exitcode", recps[i].ExitCode)
		}

		var precommitInfo miner.SectorPreCommitInfo
		if err := precommitInfo.UnmarshalCBOR(bytes.NewBuffer(m.Params)); err != nil {
			log.Warnw("failed to decode precommit params", "err", err)
			continue
		}

		refundValue, err := api.StateMinerInitialPledgeCollateral(ctx, m.To, precommitInfo, tipset.Key())
		if err != nil {
			log.Warnw("failed to calculate", "err", err)
			continue
		}

		if percentExtra > 0 {
			refundValue = types.BigAdd(refundValue, types.BigDiv(refundValue, types.NewInt(100*uint64(percentExtra))))
		}

		log.Infow("processing message", "from", m.From, "to", m.To, "value", m.Value.String(), "gas_fee_cap", m.GasFeeCap.String(), "gas_premium", m.GasPremium.String(), "gas_used", fmt.Sprintf("%d", recps[i].GasUsed), "refund", refundValue.String())

		count = count + 1
		refunds.Track(m.From, refundValue)
	}

	if refunds.Count() == 0 {
		log.Debugw("no messages to refund in tipset")
		return nil
	}

	var messages []*types.Message
	refundSum := types.NewInt(0)

	for _, maddr := range refunds.Miners() {
		refundValue := refunds.GetRefund(maddr)

		// We want to try and ensure these messages get mined quickly
		gasPremium, err := api.GasEstimateGasPremium(ctx, 0, wallet, 0, tipset.Key())
		if err != nil {
			log.Warnw("failed to estimate gas premium", "err", err)
			continue
		}

		msg := &types.Message{
			Value: refundValue,
			From:  wallet,
			To:    maddr,

			GasPremium: gasPremium,
		}

		refundSum = types.BigAdd(refundSum, msg.Value)
		messages = append(messages, msg)
	}

	balance, err := api.WalletBalance(ctx, wallet)
	if err != nil {
		return xerrors.Errorf("failed to get wallet balance :%w", err)
	}

	// Calculate the minimum balance as the total refund we need to issue plus 5% to cover fees
	minBalance := types.BigAdd(refundSum, types.BigDiv(refundSum, types.NewInt(500)))
	if balance.LessThan(minBalance) {
		log.Errorw("not sufficent funds to cover refunds", "balance", balance.String(), "refund_sum", refundSum.String(), "minimum_required", minBalance.String())
		return xerrors.Errorf("wallet does not have enough balance to cover refund")
	}

	failures := 0
	refundSum.SetUint64(0)
	for _, msg := range messages {
		if _, err = api.MpoolPushMessage(ctx, msg); err != nil {
			log.Errorw("failed to MpoolPushMessage", "err", err, "msg", msg)
			failures = failures + 1
			continue
		}

		refundSum = types.BigAdd(refundSum, msg.Value)
	}

	log.Infow("tipset stats", "messages_sent", len(messages)-failures, "refund_sum", refundSum.String(), "messages_failures", failures)

	return nil
}

type repo struct {
	last abi.ChainEpoch
	path string
}

func NewRepo(path string) (*repo, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, err
	}

	return &repo{
		last: 0,
		path: path,
	}, nil
}

func (r *repo) exists() (bool, error) {
	_, err := os.Stat(r.path)
	notexist := os.IsNotExist(err)
	if notexist {
		err = nil
	}
	return !notexist, err

}

func (r *repo) init() error {
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

func (r *repo) Open() (err error) {
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

func (r *repo) Height() abi.ChainEpoch {
	return r.last
}

func (r *repo) SetHeight(last abi.ChainEpoch) (err error) {
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
