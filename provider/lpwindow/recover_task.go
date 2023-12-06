package lpwindow

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/provider/lpmessage"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

type WdPostRecoverDeclareTask struct {
	sender       *lpmessage.Sender
	db           *harmonydb.DB
	api          WdPostRecoverDeclareTaskApi
	faultTracker sealer.FaultTracker

	maxDeclareRecoveriesGasFee types.FIL
	as                         *ctladdr.AddressSelector
	actors                     []dtypes.MinerAddress

	startCheckTF promise.Promise[harmonytask.AddTaskFunc]
}

type WdPostRecoverDeclareTaskApi interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	StateMinerPartitions(context.Context, address.Address, uint64, types.TipSetKey) ([]api.Partition, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerSectors(ctx context.Context, addr address.Address, bf *bitfield.BitField, tsk types.TipSetKey) ([]*miner.SectorOnChainInfo, error)

	GasEstimateMessageGas(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error)
	GasEstimateFeeCap(context.Context, *types.Message, int64, types.TipSetKey) (types.BigInt, error)
	GasEstimateGasPremium(_ context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error)

	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
}

func NewWdPostRecoverDeclareTask(sender *lpmessage.Sender,
	db *harmonydb.DB,
	api WdPostRecoverDeclareTaskApi,
	faultTracker sealer.FaultTracker,
	as *ctladdr.AddressSelector,
	pcs *chainsched.ProviderChainSched,

	maxDeclareRecoveriesGasFee types.FIL,
	actors []dtypes.MinerAddress) (*WdPostRecoverDeclareTask, error) {
	t := &WdPostRecoverDeclareTask{
		sender:       sender,
		db:           db,
		api:          api,
		faultTracker: faultTracker,

		maxDeclareRecoveriesGasFee: maxDeclareRecoveriesGasFee,
		as:                         as,
		actors:                     actors,
	}

	if err := pcs.AddHandler(t.processHeadChange); err != nil {
		return nil, err
	}

	return t, nil
}

func (w *WdPostRecoverDeclareTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	log.Debugw("WdPostRecoverDeclareTask.Do()", "taskID", taskID)
	ctx := context.Background()

	var spID, pps, dlIdx, partIdx uint64

	err = w.db.QueryRow(context.Background(),
		`Select sp_id, proving_period_start, deadline_index, partition_index
			from wdpost_recovery_tasks 
			where task_id = $1`, taskID).Scan(
		&spID, &pps, &dlIdx, &partIdx,
	)
	if err != nil {
		log.Errorf("WdPostRecoverDeclareTask.Do() failed to queryRow: %v", err)
		return false, err
	}

	head, err := w.api.ChainHead(context.Background())
	if err != nil {
		log.Errorf("WdPostRecoverDeclareTask.Do() failed to get chain head: %v", err)
		return false, err
	}

	deadline := wdpost.NewDeadlineInfo(abi.ChainEpoch(pps), dlIdx, head.Height())

	if deadline.FaultCutoffPassed() {
		log.Errorf("WdPostRecover removed stale task: %v %v", taskID, deadline)
		return true, nil
	}

	maddr, err := address.NewIDAddress(spID)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to NewIDAddress: %v", err)
		return false, err
	}

	partitions, err := w.api.StateMinerPartitions(context.Background(), maddr, dlIdx, head.Key())
	if err != nil {
		log.Errorf("WdPostRecoverDeclareTask.Do() failed to get partitions: %v", err)
		return false, err
	}

	if partIdx >= uint64(len(partitions)) {
		log.Errorf("WdPostRecoverDeclareTask.Do() failed to get partitions: partIdx >= len(partitions)")
		return false, err
	}

	partition := partitions[partIdx]

	unrecovered, err := bitfield.SubtractBitField(partition.FaultySectors, partition.RecoveringSectors)
	if err != nil {
		return false, xerrors.Errorf("subtracting recovered set from fault set: %w", err)
	}

	uc, err := unrecovered.Count()
	if err != nil {
		return false, xerrors.Errorf("counting unrecovered sectors: %w", err)
	}

	if uc == 0 {
		log.Warnw("nothing to declare recovered", "maddr", maddr, "deadline", deadline, "partition", partIdx)
		return true, nil
	}

	recovered, err := checkSectors(ctx, w.api, w.faultTracker, maddr, unrecovered, head.Key())
	if err != nil {
		return false, xerrors.Errorf("checking unrecovered sectors: %w", err)
	}

	// if all sectors failed to recover, don't declare recoveries
	recoveredCount, err := recovered.Count()
	if err != nil {
		return false, xerrors.Errorf("counting recovered sectors: %w", err)
	}

	if recoveredCount == 0 {
		log.Warnw("no sectors recovered", "maddr", maddr, "deadline", deadline, "partition", partIdx)
		return true, nil
	}

	recDecl := miner.RecoveryDeclaration{
		Deadline:  dlIdx,
		Partition: partIdx,
		Sectors:   recovered,
	}

	params := &miner.DeclareFaultsRecoveredParams{
		Recoveries: []miner.RecoveryDeclaration{recDecl},
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return false, xerrors.Errorf("could not serialize declare recoveries parameters: %w", aerr)
	}

	msg := &types.Message{
		To:     maddr,
		Method: builtin.MethodsMiner.DeclareFaultsRecovered,
		Params: enc,
		Value:  types.NewInt(0),
	}

	msg, mss, err := preparePoStMessage(w.api, w.as, maddr, msg, abi.TokenAmount(w.maxDeclareRecoveriesGasFee))
	if err != nil {
		return false, xerrors.Errorf("sending declare recoveries message: %w", err)
	}

	mc, err := w.sender.Send(ctx, msg, mss, "declare-recoveries")
	if err != nil {
		return false, xerrors.Errorf("sending declare recoveries message: %w", err)
	}

	log.Debugw("WdPostRecoverDeclareTask.Do() sent declare recoveries message", "maddr", maddr, "deadline", deadline, "partition", partIdx, "mc", mc)
	return true, nil
}

func (w *WdPostRecoverDeclareTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
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

func (w *WdPostRecoverDeclareTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  128,
		Name: "WdPostRecover",
		Cost: resources.Resources{
			Cpu: 1,
			Gpu: 0,
			Ram: 128 << 20,
		},
		MaxFailures: 10,
		Follows:     nil,
	}
}

func (w *WdPostRecoverDeclareTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	w.startCheckTF.Set(taskFunc)
}

func (w *WdPostRecoverDeclareTask) processHeadChange(ctx context.Context, revert, apply *types.TipSet) error {
	tf := w.startCheckTF.Val(ctx)

	for _, act := range w.actors {
		maddr := address.Address(act)

		aid, err := address.IDFromAddress(maddr)
		if err != nil {
			return xerrors.Errorf("getting miner ID: %w", err)
		}

		di, err := w.api.StateMinerProvingDeadline(ctx, maddr, apply.Key())
		if err != nil {
			return err
		}

		if !di.PeriodStarted() {
			return nil // not proving anything yet
		}

		// declaring two deadlines ahead
		declDeadline := (di.Index + 2) % di.WPoStPeriodDeadlines

		pps := di.PeriodStart
		if declDeadline != di.Index+2 {
			pps = di.NextPeriodStart()
		}

		partitions, err := w.api.StateMinerPartitions(ctx, maddr, declDeadline, apply.Key())
		if err != nil {
			return xerrors.Errorf("getting partitions: %w", err)
		}

		for pidx, partition := range partitions {
			unrecovered, err := bitfield.SubtractBitField(partition.FaultySectors, partition.RecoveringSectors)
			if err != nil {
				return xerrors.Errorf("subtracting recovered set from fault set: %w", err)
			}

			uc, err := unrecovered.Count()
			if err != nil {
				return xerrors.Errorf("counting unrecovered sectors: %w", err)
			}

			if uc == 0 {
				log.Debugw("WdPostRecoverDeclareTask.processHeadChange() uc == 0, skipping", "maddr", maddr, "declDeadline", declDeadline, "pidx", pidx)
				continue
			}

			tid := wdTaskIdentity{
				SpID:               aid,
				ProvingPeriodStart: pps,
				DeadlineIndex:      declDeadline,
				PartitionIndex:     uint64(pidx),
			}

			tf(func(id harmonytask.TaskID, tx *harmonydb.Tx) (bool, error) {
				return w.addTaskToDB(id, tid, tx)
			})
		}
	}

	return nil
}

func (w *WdPostRecoverDeclareTask) addTaskToDB(taskId harmonytask.TaskID, taskIdent wdTaskIdentity, tx *harmonydb.Tx) (bool, error) {
	_, err := tx.Exec(
		`INSERT INTO wdpost_recovery_tasks (
                         task_id,
                          sp_id,
                          proving_period_start,
                          deadline_index,
                          partition_index
                        ) VALUES ($1, $2, $3, $4, $5)`,
		taskId,
		taskIdent.SpID,
		taskIdent.ProvingPeriodStart,
		taskIdent.DeadlineIndex,
		taskIdent.PartitionIndex,
	)
	if err != nil {
		return false, xerrors.Errorf("insert partition task: %w", err)
	}

	return true, nil
}

var _ harmonytask.TaskInterface = &WdPostRecoverDeclareTask{}
