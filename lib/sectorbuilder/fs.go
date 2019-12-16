package sectorbuilder

import (
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

type dataType int

const (
	dataCache dataType = iota
	dataStaging
	dataSealed
	dataUnsealed

	nDataTypes
)

var overheadMul = []uint64{ // * sectorSize
	dataCache:    11, // TODO: check if true for 32G sectors
	dataStaging:  1,
	dataSealed:   1,
	dataUnsealed: 1,
}

type fs struct {
	path string

	// in progress actions

	reserved [nDataTypes]uint64

	lk sync.Mutex
}

func openFs(dir string) *fs {
	return &fs{
		path: dir,
	}
}

func (f *fs) init() error {
	for _, dir := range []string{f.path, f.cache(), f.staging(), f.sealed(), f.unsealed()} {
		if err := os.Mkdir(dir, 0755); err != nil {
			if os.IsExist(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func (f *fs) cache() string {
	return filepath.Join(f.path, "cache")
}

func (f *fs) staging() string {
	return filepath.Join(f.path, "staging")
}

func (f *fs) sealed() string {
	return filepath.Join(f.path, "sealed")
}

func (f *fs) unsealed() string {
	return filepath.Join(f.path, "unsealed")
}

func (f *fs) reservedBytes() int64 {
	var out int64
	for _, r := range f.reserved {
		out += int64(r)
	}
	return out
}

func (f *fs) reserve(typ dataType, size uint64) error {
	f.lk.Lock()
	defer f.lk.Unlock()

	var fsstat syscall.Statfs_t

	if err := syscall.Statfs(f.path, &fsstat); err != nil {
		return err
	}

	avail := int64(fsstat.Bavail) * fsstat.Bsize

	avail -= f.reservedBytes()

	need := overheadMul[typ] * size

	if int64(need) > avail {
		return xerrors.Errorf("not enough space in '%s', need %s, available %s", f.path, types.NewInt(need).SizeStr(), types.NewInt(uint64(avail)).SizeStr())
	}

	f.reserved[typ] += need

	return nil
}

func (f *fs) free(typ dataType, sectorSize uint64) {
	f.lk.Lock()
	defer f.lk.Unlock()

	f.reserved[typ] -= overheadMul[typ] * sectorSize

	return
}
