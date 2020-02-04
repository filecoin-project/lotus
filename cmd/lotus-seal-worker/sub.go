package main

import (
	"context"
	"net/http"

	"github.com/filecoin-project/go-sectorbuilder"
	"golang.org/x/xerrors"

	lapi "github.com/filecoin-project/lotus/api"
)

type worker struct {
	api           lapi.StorageMiner
	minerEndpoint string
	repo          string
	auth          http.Header

	limiter *limits
	sb      *sectorbuilder.SectorBuilder
}

func acceptJobs(ctx context.Context, api lapi.StorageMiner, sb *sectorbuilder.SectorBuilder, limiter *limits, endpoint string, auth http.Header, repo string, noprecommit, nocommit bool) error {
	w := &worker{
		api:           api,
		minerEndpoint: endpoint,
		auth:          auth,
		repo:          repo,

		limiter: limiter,
		sb:      sb,
	}

	tasks, err := api.WorkerQueue(ctx, sectorbuilder.WorkerCfg{
		NoPreCommit: noprecommit,
		NoCommit:    nocommit,
	})
	if err != nil {
		return err
	}

loop:
	for {
		log.Infof("Waiting for new task")

		select {
		case task := <-tasks:
			log.Infof("New task: %d, sector %d, action: %d", task.TaskID, task.SectorID, task.Type)

			res := w.processTask(ctx, task)

			log.Infof("Task %d done, err: %+v", task.TaskID, res.GoErr)

			if err := api.WorkerDone(ctx, task.TaskID, res); err != nil {
				log.Error(err)
			}
		case <-ctx.Done():
			break loop
		}
	}

	log.Warn("acceptJobs exit")
	return nil
}

func (w *worker) processTask(ctx context.Context, task sectorbuilder.WorkerTask) sectorbuilder.SealRes {
	switch task.Type {
	case sectorbuilder.WorkerPreCommit:
	case sectorbuilder.WorkerCommit:
	default:
		return errRes(xerrors.Errorf("unknown task type %d", task.Type))
	}

	if err := w.fetchSector(task.SectorID, task.Type); err != nil {
		return errRes(xerrors.Errorf("fetching sector: %w", err))
	}

	log.Infof("Data fetched, starting computation")

	var res sectorbuilder.SealRes

	switch task.Type {
	case sectorbuilder.WorkerPreCommit:
		w.limiter.workLimit <- struct{}{}
		rspco, err := w.sb.SealPreCommit(ctx, task.SectorID, task.SealTicket, task.Pieces)
		<-w.limiter.workLimit

		if err != nil {
			return errRes(xerrors.Errorf("precomitting: %w", err))
		}
		res.Rspco = rspco.ToJson()

		if err := w.push("sealed", task.SectorID); err != nil {
			return errRes(xerrors.Errorf("pushing precommited data: %w", err))
		}

		if err := w.push("cache", task.SectorID); err != nil {
			return errRes(xerrors.Errorf("pushing precommited data: %w", err))
		}

		if err := w.remove("staging", task.SectorID); err != nil {
			return errRes(xerrors.Errorf("cleaning up staged sector: %w", err))
		}
	case sectorbuilder.WorkerCommit:
		w.limiter.workLimit <- struct{}{}
		proof, err := w.sb.SealCommit(ctx, task.SectorID, task.SealTicket, task.SealSeed, task.Pieces, task.Rspco)
		<-w.limiter.workLimit

		if err != nil {
			return errRes(xerrors.Errorf("comitting: %w", err))
		}

		res.Proof = proof

		if err := w.push("cache", task.SectorID); err != nil {
			return errRes(xerrors.Errorf("pushing precommited data: %w", err))
		}

		if err := w.remove("sealed", task.SectorID); err != nil {
			return errRes(xerrors.Errorf("cleaning up sealed sector: %w", err))
		}
	}

	return res
}

func errRes(err error) sectorbuilder.SealRes {
	return sectorbuilder.SealRes{Err: err.Error(), GoErr: err}
}
