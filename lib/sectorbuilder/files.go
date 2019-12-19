package sectorbuilder

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/xerrors"
)

func (sb *SectorBuilder) SectorName(sectorID uint64) string {
	return fmt.Sprintf("s-%s-%d", sb.Miner, sectorID)
}

func (sb *SectorBuilder) StagedSectorPath(sectorID uint64) string {
	return filepath.Join(sb.filesystem.pathFor(dataStaging), sb.SectorName(sectorID))
}

func (sb *SectorBuilder) unsealedSectorPath(sectorID uint64) string {
	return filepath.Join(sb.filesystem.pathFor(dataUnsealed), sb.SectorName(sectorID))
}

func (sb *SectorBuilder) stagedSectorFile(sectorID uint64) (*os.File, error) {
	return os.OpenFile(sb.StagedSectorPath(sectorID), os.O_RDWR|os.O_CREATE, 0644)
}

func (sb *SectorBuilder) SealedSectorPath(sectorID uint64) (string, error) {
	path := filepath.Join(sb.filesystem.pathFor(dataSealed), sb.SectorName(sectorID))

	return path, nil
}

func (sb *SectorBuilder) sectorCacheDir(sectorID uint64) (string, error) {
	dir := filepath.Join(sb.filesystem.pathFor(dataCache), sb.SectorName(sectorID))

	err := os.Mkdir(dir, 0755)
	if os.IsExist(err) {
		err = nil
	}

	return dir, err
}

func (sb *SectorBuilder) GetPath(typ string, sectorName string) (string, error) {
	_, found := overheadMul[dataType(typ)]
	if !found {
		return "", xerrors.Errorf("unknown sector type: %s", typ)
	}

	return filepath.Join(sb.filesystem.pathFor(dataType(typ)), sectorName), nil
}

func (sb *SectorBuilder) TrimCache(sectorID uint64) error {
	dir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return xerrors.Errorf("getting cache dir: %w", err)
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return xerrors.Errorf("readdir: %w", err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".dat") { // _aux probably
			continue
		}
		if strings.HasSuffix(file.Name(), "-data-tree-r-last.dat") { // Want to keep
			continue
		}

		if err := os.Remove(filepath.Join(dir, file.Name())); err != nil {
			return xerrors.Errorf("rm %s: %w", file.Name(), err)
		}
	}

	return nil
}

func (sb *SectorBuilder) CanCommit(sectorID uint64) (bool, error) {
	dir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return false, xerrors.Errorf("getting cache dir: %w", err)
	}

	ents, err := ioutil.ReadDir(dir)
	if err != nil {
		return false, err
	}

	// TODO: slightly more sophisticated check
	return len(ents) == 10, nil
}

func (sb *SectorBuilder) CleanupFailedData(sectorID uint64) error {
	dir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return xerrors.Errorf("getting cache dir: %w", err)
	}

	if err := os.RemoveAll(dir); err != nil {
		return err
	}

	sealed, err := sb.SealedSectorPath(sectorID)
	if err != nil {
		return err
	}

	return os.Remove(sealed)
}

func toReadableFile(r io.Reader, n int64) (*os.File, func() error, error) {
	f, ok := r.(*os.File)
	if ok {
		return f, func() error { return nil }, nil
	}

	var w *os.File

	f, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	var wait sync.Mutex
	var werr error

	wait.Lock()
	go func() {
		defer wait.Unlock()

		var copied int64
		copied, werr = io.CopyN(w, r, n)
		if werr != nil {
			log.Warnf("toReadableFile: copy error: %+v", werr)
		}

		err := w.Close()
		if werr == nil && err != nil {
			werr = err
			log.Warnf("toReadableFile: close error: %+v", err)
			return
		}
		if copied != n {
			log.Warnf("copied different amount than expected: %d != %d", copied, n)
			werr = xerrors.Errorf("copied different amount than expected: %d != %d", copied, n)
		}
	}()

	return f, func() error {
		wait.Lock()
		return werr
	}, nil
}
