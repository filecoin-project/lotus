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
)

// FaultTracker TODO: Track things more actively
type FaultTracker interface {
	CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storiface.SectorRef, rg storiface.RGetter) (map[abi.SectorID]string, error)
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
