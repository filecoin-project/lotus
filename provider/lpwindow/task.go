package lpwindow

import (
	"context"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/storage/sealer"
	"sort"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/harmony/taskhelp"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

var log = logging.Logger("lpwindow")

var EpochsPerDeadline = miner.WPoStProvingPeriod / abi.ChainEpoch(miner.WPoStPeriodDeadlines)

type WdPostTaskDetails struct {
	Ts       *types.TipSet
	Deadline *dline.Info
}

type WDPoStAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	ChainGetTipSetAfterHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	StateMinerPartitions(context.Context, address.Address, uint64, types.TipSetKey) ([]api.Partition, error)
	StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	StateNetworkVersion(context.Context, types.TipSetKey) (network.Version, error)
	StateMinerSectors(context.Context, address.Address, *bitfield.BitField, types.TipSetKey) ([]*miner.SectorOnChainInfo, error)
}

type ProverPoSt interface {
	GenerateWindowPoStAdv(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []storiface.PostSectorChallenge, partitionIdx int, randomness abi.PoStRandomness, allowSkip bool) (storiface.WindowPoStResult, error)
}

type WdPostTask struct {
	api WDPoStAPI
	db  *harmonydb.DB

	faultTracker sealer.FaultTracker
	prover       ProverPoSt
	verifier     storiface.Verifier

	windowPoStTF promise.Promise[harmonytask.AddTaskFunc]

	actors []dtypes.MinerAddress
}

type wdTaskIdentity struct {
	Sp_id                uint64
	Proving_period_start abi.ChainEpoch
	Deadline_index       uint64
	Partition_index      uint64
}

func (t *WdPostTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {

	time.Sleep(5 * time.Second)
	log.Errorf("WdPostTask.Do() called with taskID: %v", taskID)

	var spID, pps, dlIdx, partIdx uint64

	err = t.db.QueryRow(context.Background(),
		`Select sp_id, proving_period_start, deadline_index, partition_index
			from wdpost_tasks 
			where task_id = $1`, taskID).Scan(
		&spID, &pps, &dlIdx, &partIdx,
	)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to queryRow: %v", err)
		return false, err
	}

	head, err := t.api.ChainHead(context.Background())
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to get chain head: %v", err)
		return false, err
	}

	deadline := wdpost.NewDeadlineInfo(abi.ChainEpoch(pps), dlIdx, head.Height())

	if deadline.PeriodElapsed() {
		log.Errorf("WdPost removed stale task: %v %v", taskID, deadline)
		return true, nil
	}

	maddr, err := address.NewIDAddress(spID)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to NewIDAddress: %v", err)
		return false, err
	}

	ts, err := t.api.ChainGetTipSetAfterHeight(context.Background(), deadline.Challenge, head.Key())
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to ChainGetTipSetAfterHeight: %v", err)
		return false, err
	}

	postOut, err := t.doPartition(context.Background(), ts, maddr, deadline, partIdx)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to doPartition: %v", err)
		return false, err
	}

	panic("todo record")

	_ = postOut

	/*submitWdPostParams, err := t.Scheduler.runPoStCycle(context.Background(), false, deadline, ts)
		if err != nil {
			log.Errorf("WdPostTask.Do() failed to runPoStCycle: %v", err)
			return false, err
		}

		log.Errorf("WdPostTask.Do() called with taskID: %v, submitWdPostParams: %v", taskID, submitWdPostParams)

		// Enter an entry for each wdpost message proof into the wdpost_proofs table
		for _, params := range submitWdPostParams {

			// Convert submitWdPostParams.Partitions to a byte array using CBOR
			buf := new(bytes.Buffer)
			scratch := make([]byte, 9)
			if err := cbg.WriteMajorTypeHeaderBuf(scratch, buf, cbg.MajArray, uint64(len(params.Partitions))); err != nil {
				return false, err
			}
			for _, v := range params.Partitions {
				if err := v.MarshalCBOR(buf); err != nil {
					return false, err
				}
			}

			// Insert into wdpost_proofs table
			_, err = t.db.Exec(context.Background(),
				`INSERT INTO wdpost_proofs (
	                           deadline,
	                           partitions,
	                           proof_type,
	                           proof_bytes)
	    			 VALUES ($1, $2, $3, $4)`,
				params.Deadline,
				buf.Bytes(),
				params.Proofs[0].PoStProof,
				params.Proofs[0].ProofBytes)
		}*/

	return true, nil
}

func (t *WdPostTask) CanAccept(ids []harmonytask.TaskID, te *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	// GetEpoch
	ts, err := t.api.ChainHead(context.Background())

	if err != nil {
		return nil, err
	}

	// GetData for tasks
	type wdTaskDef struct {
		Task_id              harmonytask.TaskID
		Sp_id                uint64
		Proving_period_start abi.ChainEpoch
		Deadline_index       uint64
		Partition_index      uint64

		dlInfo *dline.Info `pgx:"-"`
		openTs *types.TipSet
	}
	var tasks []wdTaskDef

	err = t.db.Select(context.Background(), &tasks,
		`Select 
			task_id,
			sp_id,
			proving_period_start,
			deadline_index,
			partition_index
	from wdpost_tasks 
	where task_id IN $1`, ids)
	if err != nil {
		return nil, err
	}

	// Accept those past deadline, then delete them in Do().
	for i := range tasks {
		tasks[i].dlInfo = wdpost.NewDeadlineInfo(tasks[i].Proving_period_start, tasks[i].Deadline_index, ts.Height())

		if tasks[i].dlInfo.PeriodElapsed() {
			return &tasks[i].Task_id, nil
		}

		tasks[i].openTs, err = t.api.ChainGetTipSetAfterHeight(context.Background(), tasks[i].dlInfo.Open, ts.Key())
		if err != nil {
			return nil, xerrors.Errorf("getting task open tipset: %w", err)
		}
	}

	// Discard those too big for our free RAM
	freeRAM := te.ResourcesAvailable().Ram
	tasks = lo.Filter(tasks, func(d wdTaskDef, _ int) bool {
		maddr, err := address.NewIDAddress(tasks[0].Sp_id)
		if err != nil {
			log.Errorf("WdPostTask.CanAccept() failed to NewIDAddress: %v", err)
			return false
		}

		mi, err := t.api.StateMinerInfo(context.Background(), maddr, ts.Key())
		if err != nil {
			log.Errorf("WdPostTask.CanAccept() failed to StateMinerInfo: %v", err)
			return false
		}

		spt, err := policy.GetSealProofFromPoStProof(mi.WindowPoStProofType)
		if err != nil {
			log.Errorf("WdPostTask.CanAccept() failed to GetSealProofFromPoStProof: %v", err)
			return false
		}

		return res[spt].MaxMemory <= freeRAM
	})
	if len(tasks) == 0 {
		log.Infof("RAM too small for any WDPost task")
		return nil, nil
	}

	// Ignore those with too many failures unless they are the only ones left.
	tasks, _ = taskhelp.SliceIfFound(tasks, func(d wdTaskDef) bool {
		var r int
		err := t.db.QueryRow(context.Background(), `SELECT COUNT(*) 
		FROM harmony_task_history 
		WHERE task_id = $1 AND success = false`, d.Task_id).Scan(&r)
		if err != nil {
			log.Errorf("WdPostTask.CanAccept() failed to queryRow: %v", err)
		}
		return r < 2
	})

	// Select the one closest to the deadline
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].dlInfo.Open < tasks[j].dlInfo.Open
	})

	return &tasks[0].Task_id, nil
}

var res = storiface.ResourceTable[sealtasks.TTGenerateWindowPoSt]

func (t *WdPostTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Name:        "WdPost",
		Max:         1, // TODO
		MaxFailures: 3,
		Follows:     nil,
		Cost: resources.Resources{
			Cpu: 1,
			Gpu: 1,
			// RAM of smallest proof's max is listed here
			Ram: lo.Reduce(lo.Keys(res), func(i uint64, k abi.RegisteredSealProof, _ int) uint64 {
				if res[k].MaxMemory < i {
					return res[k].MaxMemory
				}
				return i
			}, 1<<63),
		},
	}
}

func (t *WdPostTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	t.windowPoStTF.Set(taskFunc)
}

func (t *WdPostTask) processHeadChange(ctx context.Context, revert, apply *types.TipSet) error {
	for _, act := range t.actors {
		maddr := address.Address(act)

		aid, err := address.IDFromAddress(maddr)
		if err != nil {
			return xerrors.Errorf("getting miner ID: %w", err)
		}

		di, err := t.api.StateMinerProvingDeadline(ctx, maddr, apply.Key())
		if err != nil {
			return err
		}

		if !di.PeriodStarted() {
			return nil // not proving anything yet
		}

		partitions, err := t.api.StateMinerPartitions(ctx, maddr, di.Index, apply.Key())
		if err != nil {
			return xerrors.Errorf("getting partitions: %w", err)
		}

		// TODO: Batch Partitions??

		for pidx := range partitions {
			tid := wdTaskIdentity{
				Sp_id:                aid,
				Proving_period_start: di.PeriodStart,
				Deadline_index:       di.Index,
				Partition_index:      uint64(pidx),
			}

			tf := t.windowPoStTF.Val(ctx)
			if tf == nil {
				return xerrors.Errorf("no task func")
			}

			tf(func(id harmonytask.TaskID, tx *harmonydb.Tx) (bool, error) {
				return t.addTaskToDB(id, tid, tx)
			})
		}
	}

	return nil
}

func NewWdPostTask(db *harmonydb.DB,
	api WDPoStAPI,
	faultTracker sealer.FaultTracker,
	prover ProverPoSt,
	verifier storiface.Verifier,

	pcs *chainsched.ProviderChainSched,
	actors []dtypes.MinerAddress,
) (*WdPostTask, error) {
	t := &WdPostTask{
		db:  db,
		api: api,

		faultTracker: faultTracker,
		prover:       prover,
		verifier:     verifier,

		actors: actors,
	}

	if err := pcs.AddHandler(t.processHeadChange); err != nil {
		return nil, err
	}

	return t, nil
}

func (t *WdPostTask) addTaskToDB(taskId harmonytask.TaskID, taskIdent wdTaskIdentity, tx *harmonydb.Tx) (bool, error) {

	_, err := tx.Exec(
		`INSERT INTO wdpost_tasks (
                         task_id,
                          sp_id,
                          proving_period_start,
                          deadline_index,
                          partition_index
                        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10 , $11, $12, $13, $14)`,
		taskId,
		taskIdent.Sp_id,
		taskIdent.Proving_period_start,
		taskIdent.Deadline_index,
		taskIdent.Partition_index,
	)
	if err != nil {
		return false, err
	}

	return true, nil
}

var _ harmonytask.TaskInterface = &WdPostTask{}
