package sealing

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	market7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/market"

	"github.com/filecoin-project/lotus/chain/types"
)

func (m *Sealing) MarkForUpgrade(ctx context.Context, id abi.SectorNumber) error {
	si, err := m.GetSectorInfo(id)
	if err != nil {
		return xerrors.Errorf("getting sector info: %w", err)
	}

	if si.State != Proving {
		return xerrors.Errorf("unable to snap-up sectors not in the 'Proving' state")
	}

	if si.hasData() {
		return xerrors.Errorf("not a committed-capacity sector, has deals")
	}

	ts, err := m.Api.ChainHead(ctx)
	if err != nil {
		return xerrors.Errorf("couldn't get chain head: %w", err)
	}
	onChainInfo, err := m.Api.StateSectorGetInfo(ctx, m.maddr, id, ts.Key())
	if err != nil {
		return xerrors.Errorf("failed to read sector on chain info: %w", err)
	}
	if onChainInfo == nil {
		return xerrors.Errorf("sector not found %d", id)
	}

	active, err := m.sectorActive(ctx, ts.Key(), id)
	if err != nil {
		return xerrors.Errorf("failed to check if sector is active")
	}
	if !active {
		return xerrors.Errorf("cannot mark inactive sector for upgrade")
	}

	if onChainInfo.Expiration-ts.Height() < market7.DealMinDuration {
		return xerrors.Errorf("pointless to upgrade sector %d, expiration %d is less than a min deal duration away from current epoch."+
			"Upgrade expiration before marking for upgrade", id, onChainInfo.Expiration)
	}

	return m.sectors.Send(uint64(id), SectorMarkForUpdate{})
}

func (m *Sealing) sectorActive(ctx context.Context, tsk types.TipSetKey, sector abi.SectorNumber) (bool, error) {
	dls, err := m.Api.StateMinerDeadlines(ctx, m.maddr, tsk)
	if err != nil {
		return false, xerrors.Errorf("getting proving deadlines: %w", err)
	}

	for dl := range dls {
		parts, err := m.Api.StateMinerPartitions(ctx, m.maddr, uint64(dl), tsk)
		if err != nil {
			return false, xerrors.Errorf("getting partitions for deadline %d: %w", dl, err)
		}

		for p, part := range parts {
			set, err := part.ActiveSectors.IsSet(uint64(sector))
			if err != nil {
				return false, xerrors.Errorf("checking if sector %d is in deadline %d partition %d: %w", sector, dl, p, err)
			}
			if set {
				return true, nil
			}
		}
	}

	return false, nil
}
