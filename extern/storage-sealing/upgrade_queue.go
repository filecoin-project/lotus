package sealing

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	market7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/market"
)

func (m *Sealing) MarkForSnapUpgrade(ctx context.Context, id abi.SectorNumber) error {
	si, err := m.GetSectorInfo(id)
	if err != nil {
		return xerrors.Errorf("getting sector info: %w", err)
	}

	if si.State != Proving {
		return xerrors.Errorf("unable to snap-up sectors not in the 'Proving' state")
	}

	if si.hasDeals() {
		return xerrors.Errorf("not a committed-capacity sector, has deals")
	}

	tok, head, err := m.Api.ChainHead(ctx)
	if err != nil {
		return xerrors.Errorf("couldnt get chain head: %w", err)
	}
	onChainInfo, err := m.Api.StateSectorGetInfo(ctx, m.maddr, id, tok)
	if err != nil {
		return xerrors.Errorf("failed to read sector on chain info: %w", err)
	}

	active, err := m.sectorActive(ctx, tok, id)
	if err != nil {
		return xerrors.Errorf("failed to check if sector is active")
	}
	if !active {
		return xerrors.Errorf("cannot mark inactive sector for upgrade")
	}

	if onChainInfo.Expiration-head < market7.DealMinDuration {
		return xerrors.Errorf("pointless to upgrade sector %d, expiration %d is less than a min deal duration away from current epoch."+
			"Upgrade expiration before marking for upgrade", id, onChainInfo.Expiration)
	}

	return m.sectors.Send(uint64(id), SectorMarkForUpdate{})
}

func (m *Sealing) sectorActive(ctx context.Context, tok TipSetToken, sector abi.SectorNumber) (bool, error) {
	active, err := m.Api.StateMinerActiveSectors(ctx, m.maddr, tok)
	if err != nil {
		return false, xerrors.Errorf("failed to check active sectors: %w", err)
	}

	return active.IsSet(uint64(sector))
}
