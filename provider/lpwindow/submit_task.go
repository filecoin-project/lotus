package lpwindow

import (
	"bytes"
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/provider/lpmessage"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

type WdPoStSubmitTaskApi interface {
	ChainHead(context.Context) (*types.TipSet, error)

	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)

	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)

	GasEstimateMessageGas(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error)
	GasEstimateFeeCap(context.Context, *types.Message, int64, types.TipSetKey) (types.BigInt, error)
	GasEstimateGasPremium(_ context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error)
}

type WdPostSubmitTask struct {
	sender *lpmessage.Sender
	db     *harmonydb.DB
	api    WdPoStSubmitTaskApi

	maxWindowPoStGasFee types.FIL
	as                  *ctladdr.AddressSelector

	submitPoStTF promise.Promise[harmonytask.AddTaskFunc]
}

func NewWdPostSubmitTask(pcs *chainsched.ProviderChainSched, send *lpmessage.Sender, db *harmonydb.DB, api WdPoStSubmitTaskApi, maxWindowPoStGasFee types.FIL, as *ctladdr.AddressSelector) (*WdPostSubmitTask, error) {
	res := &WdPostSubmitTask{
		sender: send,
		db:     db,
		api:    api,

		maxWindowPoStGasFee: maxWindowPoStGasFee,
		as:                  as,
	}

	if err := pcs.AddHandler(res.processHeadChange); err != nil {
		return nil, err
	}

	return res, nil
}

func (w *WdPostSubmitTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	log.Debugw("WdPostSubmitTask.Do", "taskID", taskID)

	var spID uint64
	var deadline uint64
	var partition uint64
	var pps, submitAtEpoch, submitByEpoch abi.ChainEpoch
	var earlyParamBytes []byte
	var dbTask uint64

	err = w.db.QueryRow(
		context.Background(), `SELECT sp_id, proving_period_start, deadline, partition, submit_at_epoch, submit_by_epoch, proof_params, submit_task_id
		FROM wdpost_proofs WHERE submit_task_id = $1`, taskID,
	).Scan(&spID, &pps, &deadline, &partition, &submitAtEpoch, &submitByEpoch, &earlyParamBytes, &dbTask)
	if err != nil {
		return false, xerrors.Errorf("query post proof: %w", err)
	}

	if dbTask != uint64(taskID) {
		return false, xerrors.Errorf("taskID mismatch: %d != %d", dbTask, taskID)
	}

	head, err := w.api.ChainHead(context.Background())
	if err != nil {
		return false, xerrors.Errorf("getting chain head: %w", err)
	}

	if head.Height() > submitByEpoch {
		// we missed the deadline, no point in submitting
		log.Errorw("missed submit deadline", "spID", spID, "deadline", deadline, "partition", partition, "submitByEpoch", submitByEpoch, "headHeight", head.Height())
		return true, nil
	}

	if head.Height() < submitAtEpoch {
		log.Errorw("submit epoch not reached", "spID", spID, "deadline", deadline, "partition", partition, "submitAtEpoch", submitAtEpoch, "headHeight", head.Height())
		return false, xerrors.Errorf("submit epoch not reached: %d < %d", head.Height(), submitAtEpoch)
	}

	dlInfo := wdpost.NewDeadlineInfo(pps, deadline, head.Height())

	var params miner.SubmitWindowedPoStParams
	if err := params.UnmarshalCBOR(bytes.NewReader(earlyParamBytes)); err != nil {
		return false, xerrors.Errorf("unmarshaling proof message: %w", err)
	}

	commEpoch := dlInfo.Challenge

	commRand, err := w.api.StateGetRandomnessFromTickets(context.Background(), crypto.DomainSeparationTag_PoStChainCommit, commEpoch, nil, head.Key())
	if err != nil {
		err = xerrors.Errorf("failed to get chain randomness from tickets for windowPost (epoch=%d): %w", commEpoch, err)
		log.Errorf("submitPoStMessage failed: %+v", err)

		return false, xerrors.Errorf("getting post commit randomness: %w", err)
	}

	params.ChainCommitEpoch = commEpoch
	params.ChainCommitRand = commRand

	var pbuf bytes.Buffer
	if err := params.MarshalCBOR(&pbuf); err != nil {
		return false, xerrors.Errorf("marshaling proof message: %w", err)
	}

	maddr, err := address.NewIDAddress(spID)
	if err != nil {
		return false, xerrors.Errorf("invalid miner address: %w", err)
	}

	msg := &types.Message{
		To:     maddr,
		Method: builtin.MethodsMiner.SubmitWindowedPoSt,
		Params: pbuf.Bytes(),
		Value:  big.Zero(),
	}

	msg, mss, err := preparePoStMessage(w.api, w.as, maddr, msg, abi.TokenAmount(w.maxWindowPoStGasFee))
	if err != nil {
		return false, xerrors.Errorf("preparing proof message: %w", err)
	}

	ctx := context.Background()
	smsg, err := w.sender.Send(ctx, msg, mss, "wdpost")
	if err != nil {
		return false, xerrors.Errorf("sending proof message: %w", err)
	}

	// set message_cid in the wdpost_proofs entry

	_, err = w.db.Exec(ctx, `UPDATE wdpost_proofs SET message_cid = $1 WHERE sp_id = $2 AND proving_period_start = $3 AND deadline = $4 AND partition = $5`, smsg.String(), spID, pps, deadline, partition)
	if err != nil {
		return true, xerrors.Errorf("updating wdpost_proofs: %w", err)
	}

	return true, nil
}

func (w *WdPostSubmitTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	if len(ids) == 0 {
		// probably can't happen, but panicking is bad
		return nil, nil
	}

	if w.sender == nil {
		// we can't send messages
		return nil, nil
	}

	return &ids[0], nil
}

func (w *WdPostSubmitTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  128,
		Name: "WdPostSubmit",
		Cost: resources.Resources{
			Cpu: 0,
			Gpu: 0,
			Ram: 10 << 20,
		},
		MaxFailures: 10,
		Follows:     nil, // ??
	}
}

func (w *WdPostSubmitTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	w.submitPoStTF.Set(taskFunc)
}

func (w *WdPostSubmitTask) processHeadChange(ctx context.Context, revert, apply *types.TipSet) error {
	tf := w.submitPoStTF.Val(ctx)

	qry, err := w.db.Query(ctx, `SELECT sp_id, proving_period_start, deadline, partition, submit_at_epoch FROM wdpost_proofs WHERE submit_task_id IS NULL AND submit_at_epoch <= $1`, apply.Height())
	if err != nil {
		return err
	}
	defer qry.Close()

	for qry.Next() {
		var spID int64
		var pps int64
		var deadline uint64
		var partition uint64
		var submitAtEpoch uint64
		if err := qry.Scan(&spID, &pps, &deadline, &partition, &submitAtEpoch); err != nil {
			return xerrors.Errorf("scan submittable posts: %w", err)
		}

		tf(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, err error) {
			// update in transaction iff submit_task_id is still null
			res, err := tx.Exec(`UPDATE wdpost_proofs SET submit_task_id = $1 WHERE sp_id = $2 AND proving_period_start = $3 AND deadline = $4 AND partition = $5 AND submit_task_id IS NULL`, id, spID, pps, deadline, partition)
			if err != nil {
				return false, xerrors.Errorf("query ready proof: %w", err)
			}
			if res != 1 {
				return false, nil
			}

			return true, nil
		})
	}
	if err := qry.Err(); err != nil {
		return err
	}

	return nil
}

type MsgPrepAPI interface {
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	GasEstimateMessageGas(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error)
	GasEstimateFeeCap(context.Context, *types.Message, int64, types.TipSetKey) (types.BigInt, error)
	GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error)

	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
}

func preparePoStMessage(w MsgPrepAPI, as *ctladdr.AddressSelector, maddr address.Address, msg *types.Message, maxFee abi.TokenAmount) (*types.Message, *api.MessageSendSpec, error) {
	mi, err := w.StateMinerInfo(context.Background(), maddr, types.EmptyTSK)
	if err != nil {
		return nil, nil, xerrors.Errorf("error getting miner info: %w", err)
	}

	// set the worker as a fallback
	msg.From = mi.Worker

	mss := &api.MessageSendSpec{
		MaxFee: maxFee,
	}

	// (optimal) initial estimation with some overestimation that guarantees
	// block inclusion within the next 20 tipsets.
	gm, err := w.GasEstimateMessageGas(context.Background(), msg, mss, types.EmptyTSK)
	if err != nil {
		log.Errorw("estimating gas", "error", err)
		return nil, nil, xerrors.Errorf("estimating gas: %w", err)
	}
	*msg = *gm

	// calculate a more frugal estimation; premium is estimated to guarantee
	// inclusion within 5 tipsets, and fee cap is estimated for inclusion
	// within 4 tipsets.
	minGasFeeMsg := *msg

	minGasFeeMsg.GasPremium, err = w.GasEstimateGasPremium(context.Background(), 5, msg.From, msg.GasLimit, types.EmptyTSK)
	if err != nil {
		log.Errorf("failed to estimate minimum gas premium: %+v", err)
		minGasFeeMsg.GasPremium = msg.GasPremium
	}

	minGasFeeMsg.GasFeeCap, err = w.GasEstimateFeeCap(context.Background(), &minGasFeeMsg, 4, types.EmptyTSK)
	if err != nil {
		log.Errorf("failed to estimate minimum gas fee cap: %+v", err)
		minGasFeeMsg.GasFeeCap = msg.GasFeeCap
	}

	// goodFunds = funds needed for optimal inclusion probability.
	// minFunds  = funds needed for more speculative inclusion probability.
	goodFunds := big.Add(minGasFeeMsg.RequiredFunds(), minGasFeeMsg.Value)
	minFunds := big.Min(big.Add(minGasFeeMsg.RequiredFunds(), minGasFeeMsg.Value), goodFunds)

	from, _, err := as.AddressFor(context.Background(), w, mi, api.PoStAddr, goodFunds, minFunds)
	if err != nil {
		return nil, nil, xerrors.Errorf("error getting address: %w", err)
	}

	msg.From = from

	return msg, mss, nil
}

var _ harmonytask.TaskInterface = &WdPostSubmitTask{}
