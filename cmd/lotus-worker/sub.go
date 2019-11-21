package lotus_worker

import (
	"context"
	"golang.org/x/xerrors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

type worker struct {
	api           api.StorageMiner
	minerEndpoint string
	repo          string

	sb *sectorbuilder.SectorBuilder
}

func acceptJobs(ctx context.Context, api api.StorageMiner) error {
	w := &worker{
		api: api,
	}

	tasks, err := api.WorkerQueue(ctx)
	if err != nil {
		return err
	}

	for task := range tasks {
		res := w.processTask(ctx, task)

		api.WorkerDone(ctx)
	}
}

func (w *worker) processTask(ctx context.Context, task sectorbuilder.WorkerTask) sectorbuilder.SealRes {
	switch task.Type {
	case sectorbuilder.WorkerPreCommit:
	case sectorbuilder.WorkerCommit:
	default:
		return errRes(xerrors.Errorf("unknown task type %d", task.Type))
	}

	if err := w.fetchSector(task.SectorID, task.Type); err != nil {
		return errRes(err)
	}

	var res sectorbuilder.SealRes

	switch task.Type {
	case sectorbuilder.WorkerPreCommit:
		rspco, err := w.sb.SealPreCommit(task.SectorID, task.SealTicket, task.PublicPieceInfo)
		if err != nil {
			return errRes(err)
		}
		res.Rspco = rspco

		if err := w.push("sealed", task.SectorID); err != nil {
			return errRes(err)
		}
	case sectorbuilder.WorkerCommit:

	}

	return res
}

func (w *worker) fetch(typ string, sectorID uint64) error {
	outname := filepath.Join(w.repo, typ, w.sb.SectorName(sectorID))

	resp, err := http.Get(w.minerEndpoint + "/remote/" + typ + "/" + w.sb.SectorName(sectorID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(outname)
	if err != nil {
		return err
	}
	defer out.Close()

	// TODO: progress bar

	_, err = io.Copy(out, resp.Body)
	return err
}

func (w *worker) push(typ string, sectorID uint64) error {
	outname := filepath.Join(w.repo, typ, w.sb.SectorName(sectorID))

	f, err := os.OpenFile(outname, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", w.minerEndpoint+"/remote/"+typ+"/"+w.sb.SectorName(sectorID), f)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	return resp.Body.Close()
}

func (w *worker) fetchSector(sectorID uint64, typ sectorbuilder.WorkerTaskType) error {
	var err error
	switch typ {
	case sectorbuilder.WorkerPreCommit:
		err = w.fetch("staged", sectorID)
	case sectorbuilder.WorkerCommit:
		panic("todo")
	}
	if err != nil {
		return xerrors.Errorf("fetch failed: %w", err)
	}
	return nil
}

func errRes(err error) sectorbuilder.SealRes {
	return sectorbuilder.SealRes{Err: err}
}
