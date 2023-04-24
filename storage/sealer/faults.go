package sealer

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"

	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"

	"path/filepath"
	"os"
	"github.com/filecoin-project/specs-actors/actors/runtime/proof"
)

// FaultTracker TODO: Track things more actively
type FaultTracker interface {
	CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storiface.SectorRef, rg storiface.RGetter) (map[abi.SectorID]string, error)
	CheckProve(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storiface.SectorRef, update []bool, rg storiface.RGetter) (map[abi.SectorID]string, error)
}

// CheckProvable returns unprovable sectors
func (m *Manager) CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storiface.SectorRef, rg storiface.RGetter) (map[abi.SectorID]string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if rg == nil {
		return nil, xerrors.Errorf("rg is nil")
	}

	var bad = make(map[abi.SectorID]string)
	var badLk sync.Mutex

	var postRand abi.PoStRandomness = make([]byte, abi.RandomnessLength)
	_, _ = rand.Read(postRand)
	postRand[31] &= 0x3f

	limit := m.parallelCheckLimit
	if limit <= 0 {
		limit = len(sectors)
	}
	throttle := make(chan struct{}, limit)

	addBad := func(s abi.SectorID, reason string) {
		badLk.Lock()
		bad[s] = reason
		badLk.Unlock()
	}

	if m.partitionCheckTimeout > 0 {
		var cancel2 context.CancelFunc
		ctx, cancel2 = context.WithTimeout(ctx, m.partitionCheckTimeout)
		defer cancel2()
	}

	var wg sync.WaitGroup
	wg.Add(len(sectors))

	for _, sector := range sectors {
		select {
		case throttle <- struct{}{}:
		case <-ctx.Done():
			addBad(sector.ID, fmt.Sprintf("waiting for check worker: %s", ctx.Err()))
			wg.Done()
			continue
		}

		go func(sector storiface.SectorRef) {
			defer wg.Done()
			defer func() {
				<-throttle
			}()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			commr, update, err := rg(ctx, sector.ID)
			if err != nil {
				log.Warnw("CheckProvable Sector FAULT: getting commR", "sector", sector, "sealed", "err", err)
				addBad(sector.ID, fmt.Sprintf("getting commR: %s", err))
				return
			}

			toLock := storiface.FTSealed | storiface.FTCache
			if update {
				toLock = storiface.FTUpdate | storiface.FTUpdateCache
			}

			locked, err := m.index.StorageTryLock(ctx, sector.ID, toLock, storiface.FTNone)
			if err != nil {
				addBad(sector.ID, fmt.Sprintf("tryLock error: %s", err))
				return
			}

			if !locked {
				log.Warnw("CheckProvable Sector FAULT: can't acquire read lock", "sector", sector)
				addBad(sector.ID, fmt.Sprint("can't acquire read lock"))
				return
			}

			ch, err := ffi.GeneratePoStFallbackSectorChallenges(pp, sector.ID.Miner, postRand, []abi.SectorNumber{
				sector.ID.Number,
			})
			if err != nil {
				log.Warnw("CheckProvable Sector FAULT: generating challenges", "sector", sector, "err", err)
				addBad(sector.ID, fmt.Sprintf("generating fallback challenges: %s", err))
				return
			}

			vctx := ctx

			if m.singleCheckTimeout > 0 {
				var cancel2 context.CancelFunc
				vctx, cancel2 = context.WithTimeout(ctx, m.singleCheckTimeout)
				defer cancel2()
			}

			_, err = m.storage.GenerateSingleVanillaProof(vctx, sector.ID.Miner, storiface.PostSectorChallenge{
				SealProof:    sector.ProofType,
				SectorNumber: sector.ID.Number,
				SealedCID:    commr,
				Challenge:    ch.Challenges[sector.ID.Number],
				Update:       update,
			}, pp)
			if err != nil {
				log.Warnw("CheckProvable Sector FAULT: generating vanilla proof", "sector", sector, "err", err)
				addBad(sector.ID, fmt.Sprintf("generating vanilla proof: %s", err))
				return
			}
		}(sector)
	}

	wg.Wait()

	return bad, nil
}

var _ FaultTracker = &Manager{}

// CheckProve returns unprovable sectors
func (m *Manager) CheckProve(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storiface.SectorRef, update []bool, rg storiface.RGetter) (map[abi.SectorID]string, error) {
	var bad = make(map[abi.SectorID]string)

	ssize, err := pp.SectorSize()
	if err != nil {
		return nil, err
	}

	// TODO: More better checks
	for i, sector := range sectors {
		err := func() error {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				var fReplica string
				var fCache string

				if update[i] {
					lockedUpdate, err := m.index.StorageTryLock(ctx, sector.ID, storiface.FTUpdate|storiface.FTUpdateCache, storiface.FTNone)
					if err != nil {
						return xerrors.Errorf("acquiring sector lock: %w", err)
					}
					if !lockedUpdate {
						log.Warnw("CheckProvable Sector FAULT: can't acquire read lock on update replica", "sector", sector)
						bad[sector.ID] = fmt.Sprint("can't acquire read lock")
						return nil
					}
					lp, _, err := m.localStore.AcquireSector(ctx, sector, storiface.FTUpdate|storiface.FTUpdateCache, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
					if err != nil {
						log.Warnw("CheckProvable Sector FAULT: acquire sector update replica in checkProvable", "sector", sector, "error", err)
						bad[sector.ID] = fmt.Sprintf("acquire sector failed: %s", err)
						return nil
					}
					fReplica, fCache = lp.Update, lp.UpdateCache
				} else {
					locked, err := m.index.StorageTryLock(ctx, sector.ID, storiface.FTSealed|storiface.FTCache, storiface.FTNone)
					if err != nil {
						return xerrors.Errorf("acquiring sector lock: %w", err)
					}

					if !locked {
						log.Warnw("CheckProvable Sector FAULT: can't acquire read lock", "sector", sector)
						bad[sector.ID] = fmt.Sprint("can't acquire read lock")
						return nil
					}

					lp, _, err := m.localStore.AcquireSector(ctx, sector, storiface.FTSealed|storiface.FTCache, storiface.FTNone, storiface.PathStorage, storiface.AcquireMove)
					if err != nil {
						log.Warnw("CheckProvable Sector FAULT: acquire sector in checkProvable", "sector", sector, "error", err)
						bad[sector.ID] = fmt.Sprintf("acquire sector failed: %s", err)
						return nil
					}
					fReplica, fCache = lp.Sealed, lp.Cache
				}
				if fReplica == "" || fCache == "" {
					log.Warnw("CheckProvable Sector FAULT: cache and/or sealed paths not found", "sector", sector, "sealed", fReplica, "cache", fCache)
					bad[sector.ID] = fmt.Sprintf("cache and/or sealed paths not found, cache %q, sealed %q", fCache, fReplica)
					return nil
				}

				toCheck := map[string]int64{
					fReplica:                       1,
					filepath.Join(fCache, "p_aux"): 0,
				}

				addCachePathsForSectorSize(toCheck, fCache, ssize)

				for p, sz := range toCheck {
					st, err := os.Stat(p)
					if err != nil {
						log.Warnw("CheckProvable Sector FAULT: sector file stat error", "sector", sector, "sealed", fReplica, "cache", fCache, "file", p, "err", err)
						bad[sector.ID] = fmt.Sprintf("%s", err)
						return nil
					}

					if sz != 0 {
						if st.Size() != int64(ssize)*sz {
							log.Warnw("CheckProvable Sector FAULT: sector file is wrong size", "sector", sector, "sealed", fReplica, "cache", fCache, "file", p, "size", st.Size(), "expectSize", int64(ssize)*sz)
							bad[sector.ID] = fmt.Sprintf("%s is wrong size (got %d, expect %d)", p, st.Size(), int64(ssize)*sz)
							return nil
						}
					}
				}

				if rg != nil {
					wpp, err := sector.ProofType.RegisteredWindowPoStProof()
					if err != nil {
						return err
					}

					var pr abi.PoStRandomness = make([]byte, abi.RandomnessLength)
					_, _ = rand.Read(pr)
					pr[31] &= 0x3f

					ch, err := ffi.GeneratePoStFallbackSectorChallenges(wpp, sector.ID.Miner, pr, []abi.SectorNumber{
							sector.ID.Number,
						})
					if err != nil {
						log.Warnw("CheckProvable Sector FAULT: generating challenges", "sector", sector, "sealed", fReplica, "cache", fCache, "err", err)
						bad[sector.ID] = fmt.Sprintf("generating fallback challenges: %s", err)
						return nil
					}

					commr, _, err := rg(ctx, sector.ID)
					if err != nil {
						log.Warnw("CheckProvable Sector FAULT: getting commR", "sector", sector, "sealed", fReplica, "cache", fCache, "err", err)
						bad[sector.ID] = fmt.Sprintf("getting commR: %s", err)
						return nil
					}
					_, err = ffi.GenerateSingleVanillaProof(ffi.PrivateSectorInfo{
							SectorInfo: proof.SectorInfo{
								SealProof:    sector.ProofType,
								SectorNumber: sector.ID.Number,
								SealedCID:    commr,
							},
							CacheDirPath:     fCache,
							PoStProofType:    wpp,
							SealedSectorPath: fReplica,
						}, ch.Challenges[sector.ID.Number])
					if err != nil {
						log.Warnw("CheckProvable Sector FAULT: generating vanilla proof", "sector", sector, "sealed", fReplica, "cache", fCache, "err", err)
						bad[sector.ID] = fmt.Sprintf("generating vanilla proof: %s", err)
						return nil
					}
				}

				return nil
			}()
		if err != nil {
			return nil, err
		}
	}

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
