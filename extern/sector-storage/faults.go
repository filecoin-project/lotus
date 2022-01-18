package sectorstorage

import (
	"context"
	"crypto/rand"
	"fmt"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

// FaultTracker TODO: Track things more actively
type FaultTracker interface {
	CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storage.SectorRef, rg storiface.RGetter) (map[abi.SectorID]string, error)
}

// CheckProvable returns unprovable sectors
func (m *Manager) CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storage.SectorRef, rg storiface.RGetter) (map[abi.SectorID]string, error) {
	if rg == nil {
		return nil, xerrors.Errorf("rg is nil")
	}

	var bad = make(map[abi.SectorID]string)

	for _, sector := range sectors {
		err := func() error {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			locked, err := m.index.StorageTryLock(ctx, sector.ID, storiface.FTSealed|storiface.FTCache, storiface.FTNone)
			if err != nil {
				return xerrors.Errorf("acquiring sector lock: %w", err)
			}

			if !locked {
				log.Warnw("CheckProvable Sector FAULT: can't acquire read lock", "sector", sector)
				bad[sector.ID] = fmt.Sprint("can't acquire read lock")
				return nil
			}

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
				log.Warnw("CheckProvable Sector FAULT: generating challenges", "sector", sector, "err", err)
				bad[sector.ID] = fmt.Sprintf("generating fallback challenges: %s", err)
				return nil
			}

			commr, update, err := rg(ctx, sector.ID)
			if err != nil {
				log.Warnw("CheckProvable Sector FAULT: getting commR", "sector", sector, "sealed", "err", err)
				bad[sector.ID] = fmt.Sprintf("getting commR: %s", err)
				return nil
			}

			_, err = m.storage.GenerateSingleVanillaProof(ctx, sector.ID.Miner, storiface.PostSectorChallenge{
				SealProof:    sector.ProofType,
				SectorNumber: sector.ID.Number,
				SealedCID:    commr,
				Challenge:    ch.Challenges[sector.ID.Number],
				Update:       update,
			}, wpp)
			if err != nil {
				log.Warnw("CheckProvable Sector FAULT: generating vanilla proof", "sector", sector, "err", err)
				bad[sector.ID] = fmt.Sprintf("generating vanilla proof: %s", err)
				return nil
			}

			return nil
		}()
		if err != nil {
			return nil, err
		}
	}

	return bad, nil
}

var _ FaultTracker = &Manager{}
