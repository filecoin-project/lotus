package sectorstorage

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/sector-storage/stores"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

// TODO: Track things more actively
type FaultTracker interface {
	CheckProvable(ctx context.Context, spt abi.RegisteredProof, sectors []abi.SectorID) ([]abi.SectorID, error)
}

// Returns unprovable sectors
func (m *Manager) CheckProvable(ctx context.Context, spt abi.RegisteredProof, sectors []abi.SectorID) ([]abi.SectorID, error) {
	var bad []abi.SectorID

	// TODO: More better checks
	for _, sector := range sectors {
		err := func() error {
			lp, _, done, err := m.localStore.AcquireSector(ctx, sector, spt, stores.FTSealed|stores.FTCache, stores.FTNone, false, stores.AcquireMove)
			if err != nil {
				return xerrors.Errorf("acquire sector in checkProvable: %w", err)
			}
			defer done()

			if lp.Sealed == "" || lp.Cache == "" {
				log.Warnw("CheckProvable Sector FAULT: cache an/or sealed paths not found", "sector", sector, "sealed", lp.Sealed, "cache", lp.Cache)
				bad = append(bad, sector)
				return nil
			}

			// must be fine

			return nil
		}()
		if err != nil {
			return nil, err
		}
	}

	return bad, nil
}

var _ FaultTracker = &Manager{}
