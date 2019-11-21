package sectorbuilder

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/xerrors"
)

func (sb *SectorBuilder) SectorName(sectorID uint64) string {
	return fmt.Sprintf("s-%s-%d", sb.Miner, sectorID)
}

func (sb *SectorBuilder) StagedSectorPath(sectorID uint64) string {
	return filepath.Join(sb.stagedDir, sb.SectorName(sectorID))
}

func (sb *SectorBuilder) stagedSectorFile(sectorID uint64) (*os.File, error) {
	return os.OpenFile(sb.StagedSectorPath(sectorID), os.O_RDWR|os.O_CREATE, 0644)
}

func (sb *SectorBuilder) SealedSectorPath(sectorID uint64) (string, error) {
	path := filepath.Join(sb.sealedDir, sb.SectorName(sectorID))

	e, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return "", err
	}

	return path, e.Close()
}

func (sb *SectorBuilder) sectorCacheDir(sectorID uint64) (string, error) {
	dir := filepath.Join(sb.cacheDir, sb.SectorName(sectorID))

	err := os.Mkdir(dir, 0755)
	if os.IsExist(err) {
		err = nil
	}

	return dir, err
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

		copied, werr := io.CopyN(w, r, n)
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
