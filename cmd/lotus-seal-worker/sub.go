package main

import (
	"context"
	files "github.com/ipfs/go-ipfs-files"
	"gopkg.in/cheggaaa/pb.v1"
	"io"
	"mime"
	"mime/multipart"
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
		UnsealedDir:   filepath.Join(repo, "unsealed"),
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

		log.Infof("Task %d done, err: %+v", task.TaskID, res.GoErr)

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
		return errRes(xerrors.Errorf("fetching sector: %w", err))
	}

	var res sectorbuilder.SealRes

	switch task.Type {
	case sectorbuilder.WorkerPreCommit:
		rspco, err := w.sb.SealPreCommit(task.SectorID, task.SealTicket, task.Pieces)
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
	case sectorbuilder.WorkerCommit:
		proof, err := w.sb.SealCommit(task.SectorID, task.SealTicket, task.SealSeed, task.Pieces, task.Rspco)
		if err != nil {
			return errRes(xerrors.Errorf("comitting: %w", err))
		}

		res.Proof = proof

		if err := w.push("cache", task.SectorID); err != nil {
			return errRes(xerrors.Errorf("pushing precommited data: %w", err))
		}
	}

	return res
}

func (w *worker) fetch(typ string, sectorID uint64) error {
	outname := filepath.Join(w.repo, typ, w.sb.SectorName(sectorID))

	url := w.minerEndpoint + "/remote/" + typ + "/" + w.sb.SectorName(sectorID)
	log.Infof("Fetch %s %s", typ, url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return xerrors.Errorf("request: %w", err)
	}
	req.Header = w.auth

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return xerrors.Errorf("do request: %w", err)
	}

	defer resp.Body.Close()

	bar := pb.New64(resp.ContentLength)
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.Units = pb.U_BYTES

	barreader := bar.NewProxyReader(resp.Body)

	bar.Start()
	defer bar.Finish()

	mediatype, p, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return xerrors.Errorf("parse media type: %w", err)
	}

	var file files.Node
	switch mediatype {
	case "multipart/form-data":
		mpr := multipart.NewReader(barreader, p["boundary"])

		file, err = files.NewFileFromPartReader(mpr, mediatype)
		if err != nil {
			return xerrors.Errorf("call to NewFileFromPartReader failed: %w", err)
		}

	case "application/octet-stream":
		file = files.NewReaderFile(barreader)
	default:
		return xerrors.Errorf("unknown content type: '%s'", mediatype)
	}

	// WriteTo is unhappy when things exist
	if err := os.RemoveAll(outname); err != nil {
		return xerrors.Errorf("removing dest: %w", err)
	}

	return files.WriteTo(file, outname)
}

func (w *worker) push(typ string, sectorID uint64) error {
	outname := filepath.Join(w.repo, typ, w.sb.SectorName(sectorID))

	stat, err := os.Stat(outname)
	if err != nil {
		return err
	}

	f, err := files.NewSerialFile(outname, false, stat)
	if err != nil {
		return err
	}

	url := w.minerEndpoint + "/remote/" + typ + "/" + w.sb.SectorName(sectorID)
	log.Infof("Push %s %s", typ, url)

	sz, err := f.Size()
	if err != nil {
		return xerrors.Errorf("getting size: %w", err)
	}

	bar := pb.New64(sz)
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.Units = pb.U_BYTES

	bar.Start()
	defer bar.Finish()
	//todo set content size

	header := w.auth

	var r io.Reader
	r, file := f.(files.File)
	if !file {
		mfr := files.NewMultiFileReader(f.(files.Directory), true)

		header.Set("Content-Type", "multipart/form-data; boundary="+mfr.Boundary())
		r = mfr
	} else {
		header.Set("Content-Type", "application/octet-stream")
	}

	req, err := http.NewRequest("PUT", url, bar.NewProxyReader(r))
	if err != nil {
		return err
	}
	req.Header = header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return xerrors.Errorf("non-200 response: %d", resp.StatusCode)
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
		if err != nil {
			return xerrors.Errorf("fetch sealed: %w", err)
		}
		err = w.fetch("cache", sectorID)
	}
	if err != nil {
		return xerrors.Errorf("fetch failed: %w", err)
	}
	return nil
}

func errRes(err error) sectorbuilder.SealRes {
	return sectorbuilder.SealRes{Err: err.Error(), GoErr: err}
}
