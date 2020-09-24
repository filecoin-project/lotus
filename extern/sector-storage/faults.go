package sectorstorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
)

// FaultTracker TODO: Track things more actively
type FaultTracker interface {
	CheckProvable(ctx context.Context, spt abi.RegisteredSealProof, sectors []abi.SectorID) ([]abi.SectorID, error)
}

// CheckProvable returns unprovable sectors
func (m *Manager) CheckProvable(ctx context.Context, spt abi.RegisteredSealProof, sectors []abi.SectorID) ([]abi.SectorID, error) {
	var bad []abi.SectorID

	ssize, err := spt.SectorSize()
	if err != nil {
		return nil, err
	}

	// TODO: More better checks
	toCheck := make(chan abi.SectorID, len(sectors))
	bads := make(chan abi.SectorID, len(sectors))
	var threads int
	if len(sectors) > runtime.NumCPU()*2 {
		threads = runtime.NumCPU() * 2
	} else {
		threads = len(sectors)
	}
	wg := sync.WaitGroup{}

	start := time.Now()
	log.Infof("use %d threads to check %d sectors", threads, len(sectors))
	for w := 0; w < threads; w++ {
		go func() {
			wg.Add(1)
			defer wg.Done()
			for sector := range toCheck {
				lp, _, err := m.localStore.AcquireSector(ctx, sector, spt, stores.FTSealed|stores.FTCache, stores.FTNone, stores.PathStorage, stores.AcquireMove)
				if err != nil {
					log.Warnw("CheckProvable Sector FAULT: acquire sector in checkProvable", "sector", sector, "error", err)
					bads <- sector
					continue
				}

				if lp.Sealed == "" || lp.Cache == "" {
					log.Warnw("CheckProvable Sector FAULT: cache an/or sealed paths not found", "sector", sector, "sealed", lp.Sealed, "cache", lp.Cache)
					bads <- sector
					continue
				}

				toCheck := map[string]int64{
					lp.Sealed:                        1,
					filepath.Join(lp.Cache, "t_aux"): 0,
					filepath.Join(lp.Cache, "p_aux"): 0,
				}

				addCachePathsForSectorSize(toCheck, lp.Cache, ssize)

				for p, sz := range toCheck {
					st, err := os.Stat(p)
					if err != nil {
						log.Warnw("CheckProvable Sector FAULT: sector file stat error", "sector", sector, "sealed", lp.Sealed, "cache", lp.Cache, "file", p, "err", err)
						bads <- sector
						break
					}

					if sz != 0 {
						if st.Size() != int64(ssize)*sz {
							log.Warnw("CheckProvable Sector FAULT: sector file is wrong size", "sector", sector, "sealed", lp.Sealed, "cache", lp.Cache, "file", p, "size", st.Size(), "expectSize", int64(ssize)*sz)
							bads <- sector
							break
						}
					}
				}
			}
		}()
	}

	for _, sector := range sectors {
		locked, err := m.index.StorageTryLock(ctx, sector, stores.FTSealed|stores.FTCache, stores.FTNone)
		if err != nil {
			return bad, xerrors.Errorf("acquiring sector lock: %w", err)
		}

		if !locked {
			log.Warnw("CheckProvable Sector FAULT: can't acquire read lock", "sector", sector, "sealed")
			bads <- sector
		}
		toCheck <- sector
	}
	close(toCheck)

	wg.Wait()
	close(bads)
	for sector := range bads {
		bad = append(bad, sector)
	}
	log.Infof("checking of %d sectors is finished. elapsed: %d", len(sectors), time.Since(start).Milliseconds())

	return bad, nil
}

func addCachePathsForSectorSize(chk map[string]int64, cacheDir string, ssize abi.SectorSize) {
	switch ssize {
	case 2 << 10:
		fallthrough
	case 8 << 20:
		fallthrough
	case 512 << 20:
		chk[filepath.Join(cacheDir, "sc-02-data-tree-r-last.dat")] = 0
	case 32 << 30:
		for i := 0; i < 8; i++ {
			chk[filepath.Join(cacheDir, fmt.Sprintf("sc-02-data-tree-r-last-%d.dat", i))] = 0
		}
	case 64 << 30:
		for i := 0; i < 16; i++ {
			chk[filepath.Join(cacheDir, fmt.Sprintf("sc-02-data-tree-r-last-%d.dat", i))] = 0
		}
	default:
		log.Warnf("not checking cache files of %s sectors for faults", ssize)
	}
}

var _ FaultTracker = &Manager{}
