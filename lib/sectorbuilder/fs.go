package sectorbuilder

import (
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

type dataType string

const (
	dataCache    dataType = "cache"
	dataStaging  dataType = "staging"
	dataSealed   dataType = "sealed"
	dataUnsealed dataType = "unsealed"
)

var overheadMul = map[dataType]uint64{ // * sectorSize
	dataCache:    11, // TODO: check if true for 32G sectors
	dataStaging:  1,
	dataSealed:   1,
	dataUnsealed: 1,
}

type fs struct {
	path string

	// in progress actions

	reserved map[dataType]uint64

	lk sync.Mutex
}

func openFs(dir string) *fs {
	return &fs{
		path:     dir,
		reserved: map[dataType]uint64{},
	}
}

func (f *fs) init() error {
	for _, dir := range []string{f.path,
		f.pathFor(dataCache),
		f.pathFor(dataStaging),
		f.pathFor(dataSealed),
		f.pathFor(dataUnsealed)} {
		if err := os.Mkdir(dir, 0755); err != nil {
			if os.IsExist(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func (f *fs) pathFor(typ dataType) string {
	_, found := overheadMul[typ]
	if !found {
		panic("unknown data path requested")
	}

	return filepath.Join(f.path, string(typ))
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

	if err := syscall.Statfs(f.pathFor(typ), &fsstat); err != nil {
		return err
	}

	fsavail := int64(fsstat.Bavail) * int64(fsstat.Bsize)

	avail := fsavail - f.reservedBytes()

	need := overheadMul[typ] * size

	if int64(need) > avail {
		return xerrors.Errorf("not enough space in '%s', need %s, available %s (fs: %s, reserved: %s)",
			f.path,
			types.NewInt(need).SizeStr(),
			types.NewInt(uint64(avail)).SizeStr(),
			types.NewInt(uint64(fsavail)).SizeStr(),
			types.NewInt(uint64(f.reservedBytes())))
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
