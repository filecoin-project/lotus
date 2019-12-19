package main

import (
	"context"
	"github.com/google/uuid"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

var constRemoteID = ""

type worker struct {
	api           lapi.StorageMiner
	minerEndpoint string
	repo          string
	auth          http.Header

	sb *sectorbuilder.SectorBuilder
}


//host ip
func get_hostip() string {
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					log.Infof("get_hostip:%s", ipnet.IP.String())
					return ipnet.IP.String()
				}
			}
		}
	}

	remoteid ,_ := uuid.NewRandom()
	return remoteid.String()
}

func acceptJobs(ctx context.Context, api lapi.StorageMiner, endpoint string, auth http.Header, repo string, noprecommit, nocommit bool, nosealone bool) error {
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
		Dir:           repo,
	})
	if err != nil {
		return err
	}

	if err := build.GetParams(ssize); err != nil {
		return xerrors.Errorf("get params: %w", err)
	}

	constRemoteID = get_hostip()
	if nosealone {
		constRemoteID = ""
	}

	w := &worker{
		api:           api,
		minerEndpoint: endpoint,
		auth:          auth,
		repo:          repo,
		sb:            sb,
	}

	tasks, err := api.WorkerQueue(ctx, sectorbuilder.WorkerCfg{
		NoPreCommit: noprecommit,
		NoCommit:    nocommit,
		RemoteID:    constRemoteID,
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
			log.Infof("WorkerCfg: %s RemoteID: %s", constRemoteID, task.RemoteID)
			seal := time.Now()
			res := w.processTask(ctx, task)

			if err := api.WorkerDone(ctx, task.TaskID, res); err != nil {
				log.Error(err)
			}

			log.Infof("bench Task %d done,  task.Type %d use %s , err: %+v", task.TaskID,  task.Type, time.Since(seal), res.GoErr)

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
		rspco, _, err := w.sb.SealPreCommit(task.SectorID, task.SealTicket, task.Pieces)
		if err != nil {
			return errRes(xerrors.Errorf("precomitting: %w", err))
		}
		res.Rspco = rspco.ToJson()

		res.Rspco.RemoteID = constRemoteID

		if err := w.push("sealed", task.SectorID); err != nil {
			return errRes(xerrors.Errorf("pushing precommited data: %w", err))
		}

		cachename := filepath.Join(w.repo, "cache", w.sb.SectorName(task.SectorID))
		bakname := filepath.Join(w.repo, "cache", w.sb.SectorName(task.SectorID) + "_bak")

        //移除文件
		os.Mkdir(bakname, 750)

		os.Rename(filepath.Join(cachename, "sc-01-data-layer-0.dat"),  filepath.Join(bakname, "sc-01-data-layer-0.dat"))
		os.Rename(filepath.Join(cachename, "sc-01-data-layer-1.dat"),  filepath.Join(bakname, "sc-01-data-layer-1.dat"))
		os.Rename(filepath.Join(cachename, "sc-01-data-layer-2.dat"),  filepath.Join(bakname, "sc-01-data-layer-2.dat"))
		os.Rename(filepath.Join(cachename, "sc-01-data-layer-3.dat"),  filepath.Join(bakname, "sc-01-data-layer-3.dat"))

		os.Rename(filepath.Join(cachename, "sc-01-data-tree-c.dat"),  filepath.Join(bakname, "sc-01-data-tree-c.dat"))
		os.Rename(filepath.Join(cachename, "sc-01-data-tree-d.dat"),  filepath.Join(bakname, "sc-01-data-tree-d.dat"))
		os.Rename(filepath.Join(cachename, "sc-01-data-tree-q.dat"),  filepath.Join(bakname, "sc-01-data-tree-q.dat"))

		if err := w.push("cache", task.SectorID); err != nil {
			return errRes(xerrors.Errorf("pushing precommited data: %w", err))
		}

		os.Rename(filepath.Join(bakname, "sc-01-data-layer-0.dat"),  filepath.Join(cachename, "sc-01-data-layer-0.dat"))
		os.Rename(filepath.Join(bakname, "sc-01-data-layer-1.dat"),  filepath.Join(cachename, "sc-01-data-layer-1.dat"))
		os.Rename(filepath.Join(bakname, "sc-01-data-layer-2.dat"),  filepath.Join(cachename, "sc-01-data-layer-2.dat"))
		os.Rename(filepath.Join(bakname, "sc-01-data-layer-3.dat"),  filepath.Join(cachename, "sc-01-data-layer-3.dat"))

		os.Rename(filepath.Join(bakname, "sc-01-data-tree-c.dat"),  filepath.Join(cachename, "sc-01-data-tree-c.dat"))
		os.Rename(filepath.Join(bakname, "sc-01-data-tree-d.dat"),  filepath.Join(cachename, "sc-01-data-tree-d.dat"))
		os.Rename(filepath.Join(bakname, "sc-01-data-tree-q.dat"),  filepath.Join(cachename, "sc-01-data-tree-q.dat"))

		os.RemoveAll(bakname)

	case sectorbuilder.WorkerCommit:
		proof, err := w.sb.SealCommitLocal(task.SectorID, task.SealTicket, task.SealSeed, task.Pieces, task.Rspco)
		//if err := w.push("cache", task.SectorID); err != nil {
		//	return errRes(xerrors.Errorf("pushing precommited data: %w", err))
		//}
		 cachefilename := filepath.Join(w.repo, "cache", w.sb.SectorName(task.SectorID))
		 os.RemoveAll(cachefilename)

		sealedfilename := filepath.Join(w.repo, "sealed", w.sb.SectorName(task.SectorID))
		os.RemoveAll(sealedfilename)

		stagingfilename := filepath.Join(w.repo, "staging", w.sb.SectorName(task.SectorID))
		os.RemoveAll(stagingfilename)
		if err != nil {
			return errRes(xerrors.Errorf("comitting: %w", err))
		}

		res.Proof = proof

	}

	return res
}

func errRes(err error) sectorbuilder.SealRes {
	return sectorbuilder.SealRes{Err: err.Error(), GoErr: err}
}
