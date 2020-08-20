package basicfs

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

type sectorFile struct {
	abi.SectorID
	stores.SectorFileType
}

type Provider struct {
	Root string

	lk         sync.Mutex
	waitSector map[sectorFile]chan struct{}
}

func (b *Provider) AcquireSector(ctx context.Context, id abi.SectorID, existing stores.SectorFileType, allocate stores.SectorFileType, ptype stores.PathType) (stores.SectorPaths, func(), error) {
	if err := os.Mkdir(filepath.Join(b.Root, stores.FTUnsealed.String()), 0755); err != nil && !os.IsExist(err) { // nolint
		return stores.SectorPaths{}, nil, err
	}
	if err := os.Mkdir(filepath.Join(b.Root, stores.FTSealed.String()), 0755); err != nil && !os.IsExist(err) { // nolint
		return stores.SectorPaths{}, nil, err
	}
	if err := os.Mkdir(filepath.Join(b.Root, stores.FTCache.String()), 0755); err != nil && !os.IsExist(err) { // nolint
		return stores.SectorPaths{}, nil, err
	}

	done := func() {}

	out := stores.SectorPaths{
		ID: id,
	}

	for _, fileType := range stores.PathTypes {
		if !existing.Has(fileType) && !allocate.Has(fileType) {
			continue
		}

		b.lk.Lock()
		if b.waitSector == nil {
			b.waitSector = map[sectorFile]chan struct{}{}
		}
		ch, found := b.waitSector[sectorFile{id, fileType}]
		if !found {
			ch = make(chan struct{}, 1)
			b.waitSector[sectorFile{id, fileType}] = ch
		}
		b.lk.Unlock()

		select {
		case ch <- struct{}{}:
		case <-ctx.Done():
			done()
			return stores.SectorPaths{}, nil, ctx.Err()
		}

		path := filepath.Join(b.Root, fileType.String(), stores.SectorName(id))

		prevDone := done
		done = func() {
			prevDone()
			<-ch
		}

		if !allocate.Has(fileType) {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				done()
				return stores.SectorPaths{}, nil, storiface.ErrSectorNotFound
			}
		}

		stores.SetPathByType(&out, fileType, path)
	}

	return out, done, nil
}
