package sectorbuilder

import (
	"context"

	"golang.org/x/xerrors"
)

type WorkerTaskType int

const (
	WorkerPreCommit WorkerTaskType = iota
	WorkerCommit
)

type WorkerTask struct {
	Type   WorkerTaskType
	TaskID uint64

	SectorID uint64

	// preCommit
	SealTicket SealTicket
	Pieces     []PublicPieceInfo

	// commit
	SealSeed SealSeed
	Rspco    RawSealPreCommitOutput
}

type workerCall struct {
	task WorkerTask
	ret  chan SealRes
}

func (sb *SectorBuilder) AddWorker(ctx context.Context, cfg WorkerCfg) (<-chan WorkerTask, error) {
	sb.remoteLk.Lock()
	defer sb.remoteLk.Unlock()

	taskCh := make(chan WorkerTask)
	r := &remote{
		sealTasks: taskCh,
		busy:      0,
	}

	sb.remoteCtr++
	sb.remotes[sb.remoteCtr] = r

	go sb.remoteWorker(ctx, r, cfg)

	return taskCh, nil
}

func (sb *SectorBuilder) returnTask(task workerCall) {
	var ret chan workerCall
	switch task.task.Type {
	case WorkerPreCommit:
		ret = sb.precommitTasks
	case WorkerCommit:
		ret = sb.commitTasks
	default:
		log.Error("unknown task type", task.task.Type)
	}

	go func() {
		select {
		case ret <- task:
		case <-sb.stopping:
			return
		}
	}()
}

func (sb *SectorBuilder) remoteWorker(ctx context.Context, r *remote, cfg WorkerCfg) {
	defer log.Warn("Remote worker disconnected")

	defer func() {
		sb.remoteLk.Lock()
		defer sb.remoteLk.Unlock()

		for i, vr := range sb.remotes {
			if vr == r {
				delete(sb.remotes, i)
				return
			}
		}
	}()

	precommits := sb.precommitTasks
	if cfg.NoPreCommit {
		precommits = nil
	}
	commits := sb.commitTasks
	if cfg.NoCommit {
		commits = nil
	}

	for {
		select {
		case task := <-commits:
			sb.doTask(ctx, r, task)
		case task := <-precommits:
			sb.doTask(ctx, r, task)
		case <-ctx.Done():
			return
		case <-sb.stopping:
			return
		}

		r.lk.Lock()
		r.busy = 0
		r.lk.Unlock()
	}
}

func (sb *SectorBuilder) doTask(ctx context.Context, r *remote, task workerCall) {
	resCh := make(chan SealRes)

	sb.remoteLk.Lock()
	sb.remoteResults[task.task.TaskID] = resCh
	sb.remoteLk.Unlock()

	// send the task
	select {
	case r.sealTasks <- task.task:
	case <-ctx.Done():
		sb.returnTask(task)
		return
	}

	r.lk.Lock()
	r.busy = task.task.TaskID
	r.lk.Unlock()

	// wait for the result
	select {
	case res := <-resCh:

		// send the result back to the caller
		select {
		case task.ret <- res:
		case <-ctx.Done():
			return
		case <-sb.stopping:
			return
		}

	case <-ctx.Done():
		log.Warnf("context expired while waiting for task %d (sector %d): %s", task.task.TaskID, task.task.SectorID, ctx.Err())
		return
	case <-sb.stopping:
		return
	}
}

func (sb *SectorBuilder) TaskDone(ctx context.Context, task uint64, res SealRes) error {
	sb.remoteLk.Lock()
	rres, ok := sb.remoteResults[task]
	if ok {
		delete(sb.remoteResults, task)
	}
	sb.remoteLk.Unlock()

	if !ok {
		return xerrors.Errorf("task %d not found", task)
	}

	select {
	case rres <- res:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
