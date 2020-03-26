package basicfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/storage/sectorstorage/stores"
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

func (b *Provider) AcquireSector(ctx context.Context, id abi.SectorID, existing stores.SectorFileType, allocate stores.SectorFileType, sealing bool) (stores.SectorPaths, func(), error) {
	os.Mkdir(filepath.Join(b.Root, stores.FTUnsealed.String()), 0755)
	os.Mkdir(filepath.Join(b.Root, stores.FTSealed.String()), 0755)
	os.Mkdir(filepath.Join(b.Root, stores.FTCache.String()), 0755)

	done := func() {}

	for i := 0; i < 3; i++ {
		if (existing|allocate)&(1<<i) == 0 {
			continue
		}

		b.lk.Lock()
		if b.waitSector == nil {
			b.waitSector = map[sectorFile]chan struct{}{}
		}
		ch, found := b.waitSector[sectorFile{id, 1 << i}]
		if !found {
			ch = make(chan struct{}, 1)
			b.waitSector[sectorFile{id, 1 << i}] = ch
		}
		b.lk.Unlock()

		select {
		case ch <- struct{}{}:
		case <-ctx.Done():
			done()
			return stores.SectorPaths{}, nil, ctx.Err()
		}

		prevDone := done
		done = func() {
			prevDone()
			<-ch
		}
	}

	return stores.SectorPaths{
		Id:       id,
		Unsealed: filepath.Join(b.Root, stores.FTUnsealed.String(), fmt.Sprintf("s-t0%d-%d", id.Miner, id.Number)),
		Sealed:   filepath.Join(b.Root, stores.FTSealed.String(), fmt.Sprintf("s-t0%d-%d", id.Miner, id.Number)),
		Cache:    filepath.Join(b.Root, stores.FTCache.String(), fmt.Sprintf("s-t0%d-%d", id.Miner, id.Number)),
	}, done, nil
}
