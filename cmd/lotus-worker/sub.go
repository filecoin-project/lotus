package main

import (
	"context"
	"gopkg.in/cheggaaa/pb.v1"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

type worker struct {
	api           api.StorageMiner
	minerEndpoint string
	repo          string
	auth          http.Header

	sb *sectorbuilder.SectorBuilder
}

func acceptJobs(ctx context.Context, api api.StorageMiner, endpoint string, auth http.Header, repo string) error {
	act, err := api.ActorAddress(ctx)
	if err != nil {
		return err
	}
	ssize, err := api.ActorSectorSize(ctx, act)
	if err != nil {
		return err
	}

	sb, err := sectorbuilder.NewStandalone(&sectorbuilder.Config{
		SectorSize:    ssize,
		Miner:         act,
		WorkerThreads: 1,
		CacheDir:      filepath.Join(repo, "cache"),
		SealedDir:     filepath.Join(repo, "sealed"),
		StagedDir:     filepath.Join(repo, "staged"),
		MetadataDir:   filepath.Join(repo, "meta"),
	})
	if err != nil {
		return err
	}

	w := &worker{
		api:           api,
		minerEndpoint: endpoint,
		auth:          auth,
		repo:          repo,
		sb:            sb,
	}

	tasks, err := api.WorkerQueue(ctx)
	if err != nil {
		return err
	}

	for task := range tasks {
		log.Infof("New task: %d, sector %d, action: %d", task.TaskID, task.SectorID, task.Type)

		res := w.processTask(ctx, task)

		log.Infof("Task %d done, err: %s", task.TaskID, res.Err)

		if err := api.WorkerDone(ctx, task.TaskID, res); err != nil {
			log.Error(err)
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
		return errRes(err)
	}

	var res sectorbuilder.SealRes

	switch task.Type {
	case sectorbuilder.WorkerPreCommit:
		rspco, err := w.sb.SealPreCommit(task.SectorID, task.SealTicket, task.Pieces)
		if err != nil {
			return errRes(err)
		}
		res.Rspco = rspco

		// TODO: push cache

		if err := w.push("sealed", task.SectorID); err != nil {
			return errRes(err)
		}
	case sectorbuilder.WorkerCommit:
		proof, err := w.sb.SealCommit(task.SectorID, task.SealTicket, task.SealSeed, task.Pieces, nil, task.Rspco)
		if err != nil {
			return errRes(err)
		}

		res.Proof = proof

		// TODO: Push cache
	}

	return res
}

func (w *worker) fetch(typ string, sectorID uint64) error {
	outname := filepath.Join(w.repo, typ, w.sb.SectorName(sectorID))

	url := w.minerEndpoint + "/remote/" + typ + "/" + w.sb.SectorName(sectorID)
	log.Infof("Fetch %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header = w.auth

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	out, err := os.Create(outname)
	if err != nil {
		return err
	}
	defer out.Close()

	bar := pb.New64(resp.ContentLength)
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.Units = pb.U_BYTES

	bar.Start()
	defer bar.Finish()

	_, err = io.Copy(out, bar.NewProxyReader(resp.Body))
	return err
}

func (w *worker) push(typ string, sectorID uint64) error {
	outname := filepath.Join(w.repo, typ, w.sb.SectorName(sectorID))

	f, err := os.OpenFile(outname, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}

	url := w.minerEndpoint + "/remote/" + typ + "/" + w.sb.SectorName(sectorID)
	log.Infof("Push %s", url)

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	bar := pb.New64(fi.Size())
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.Units = pb.U_BYTES

	bar.Start()
	defer bar.Finish()
	//todo set content size
	req, err := http.NewRequest("PUT", url, bar.NewProxyReader(f))
	if err != nil {
		return err
	}
	req.Header = w.auth

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return xerrors.Errorf("non-200 response: %d", resp.StatusCode)
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
		err = w.fetch("sealed", sectorID)
		// todo: cache
	}
	if err != nil {
		return xerrors.Errorf("fetch failed: %w", err)
	}
	return nil
}

func errRes(err error) sectorbuilder.SealRes {
	return sectorbuilder.SealRes{Err: err}
}
