package sealing

import (
	"context"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/specs-storage/storage"

	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
)

func (m *Sealing) handleWaitDeals(ctx statemachine.Context, sector SectorInfo) error {
	// if full / oldish / has oldish deals goto seal
	// ^ also per sector deal limit

	m.inputLk.Lock()

	now := time.Now()
	st := m.sectorTimers[m.minerSectorID(sector.SectorNumber)]
	if st != nil {
		if !st.Stop() { // timer expired, SectorStartPacking was/is being sent
			m.inputLk.Unlock()

			// we send another SectorStartPacking in case one was sent in the handleAddPiece state
			return ctx.Send(SectorStartPacking{})
		}
	}

	if !sector.CreationTime.IsZero() {
		cfg, err := m.getConfig()
		if err != nil {
			m.inputLk.Unlock()
			return xerrors.Errorf("getting storage config: %w", err)
		}

		sealTime := sector.CreationTime.Add(cfg.WaitDealsDelay)
		if now.After(sealTime) {
			m.inputLk.Unlock()
			return ctx.Send(SectorStartPacking{})
		}

		m.sectorTimers[m.minerSectorID(sector.SectorNumber)] = time.AfterFunc(sealTime.Sub(now), func() {
			if err := ctx.Send(SectorStartPacking{}); err != nil {
				log.Errorw("sending SectorStartPacking event failed", "sector", sector.SectorNumber, "error", err)
			}
		})
	}

	var used abi.UnpaddedPieceSize
	for _, piece := range sector.Pieces {
		used += piece.Piece.Size.Unpadded()
	}

	m.openSectors[m.minerSectorID(sector.SectorNumber)] = &openSector{
		used: used,
		maybeAccept: func(cid cid.Cid) error {
			// todo double check space

			// todo check deal expiration

			return ctx.Send(SectorAddPiece{cid})
		},
	}

	go func() {
		defer m.inputLk.Unlock()
		if err := m.updateInput(ctx.Context(), sector.SectorType); err != nil {
			log.Errorf("%+v", err)
		}
	}()

	return nil
}

func (m *Sealing) handleAddPiece(ctx statemachine.Context, sector SectorInfo) error {
	ssize, err := sector.SectorType.SectorSize()
	if err != nil {
		return err
	}

	m.inputLk.Lock()
	delete(m.openSectors, m.minerSectorID(sector.SectorNumber)) // todo: do this when handling the event
	m.inputLk.Unlock()

	res := SectorPieceAdded{}

	var offset abi.UnpaddedPieceSize
	pieceSizes := make([]abi.UnpaddedPieceSize, len(sector.Pieces))
	for i, p := range sector.Pieces {
		pieceSizes[i] = p.Piece.Size.Unpadded()
		offset += p.Piece.Size.Unpadded()
	}

	for _, piece := range sector.PendingPieces {
		m.inputLk.Lock()
		deal, ok := m.pendingPieces[piece]
		m.inputLk.Unlock()
		if !ok {
			// todo: this probably means that the miner process was restarted in the middle of adding pieces.
			//  Truncate whatever was in process of being added to the sector (keep sector.Pieces as those are cleanly added, then go to WaitDeals)
			return xerrors.Errorf("piece %s assigned to sector %d not found", piece, sector.SectorNumber)
		}

		pads, padLength := ffiwrapper.GetRequiredPadding(offset.Padded(), deal.size.Padded())

		if offset.Padded()+padLength+deal.size.Padded() > abi.PaddedPieceSize(ssize) {
			return xerrors.Errorf("piece assigned to a sector with not enough space")
		}

		offset += padLength.Unpadded()

		for _, p := range pads {
			ppi, err := m.sealer.AddPiece(sectorstorage.WithPriority(ctx.Context(), DealSectorPriority),
				m.minerSector(sector.SectorType, sector.SectorNumber),
				pieceSizes,
				p.Unpadded(),
				NewNullReader(p.Unpadded()))
			if err != nil {
				return xerrors.Errorf("writing padding piece: %w", err) // todo failed state
			}

			pieceSizes = append(pieceSizes, p.Unpadded())
			res.NewPieces = append(res.NewPieces, Piece{
				Piece: ppi,
			})
		}

		ppi, err := m.sealer.AddPiece(sectorstorage.WithPriority(ctx.Context(), DealSectorPriority),
			m.minerSector(sector.SectorType, sector.SectorNumber),
			pieceSizes,
			deal.size,
			deal.data)
		if err != nil {
			return xerrors.Errorf("writing padding piece: %w", err) // todo failed state
		}

		deal.accepted(sector.SectorNumber, offset, nil)

		offset += deal.size
		pieceSizes = append(pieceSizes, deal.size)

		res.NewPieces = append(res.NewPieces, Piece{
			Piece:    ppi,
			DealInfo: &deal.deal,
		})
	}

	return ctx.Send(res)
}

func (m *Sealing) AddPieceToAnySector(ctx context.Context, size abi.UnpaddedPieceSize, data storage.Data, deal DealInfo) (abi.SectorNumber, abi.PaddedPieceSize, error) {
	log.Infof("Adding piece for deal %d (publish msg: %s)", deal.DealID, deal.PublishCid)
	if (padreader.PaddedSize(uint64(size))) != size {
		return 0, 0, xerrors.Errorf("cannot allocate unpadded piece")
	}

	sp, err := m.currentSealProof(ctx)
	if err != nil {
		return 0, 0, xerrors.Errorf("getting current seal proof type: %w", err)
	}

	ssize, err := sp.SectorSize()
	if err != nil {
		return 0, 0, err
	}

	if size > abi.PaddedPieceSize(ssize).Unpadded() {
		return 0, 0, xerrors.Errorf("piece cannot fit into a sector")
	}

	if deal.PublishCid == nil {
		return 0, 0, xerrors.Errorf("piece must have a PublishCID")
	}

	m.inputLk.Lock()
	if _, exist := m.pendingPieces[*deal.PublishCid]; exist {
		m.inputLk.Unlock()
		return 0, 0, xerrors.Errorf("piece for deal %s already pending", *deal.PublishCid)
	}

	resCh := make(chan struct {
		sn     abi.SectorNumber
		offset abi.UnpaddedPieceSize
		err    error
	}, 1)

	m.pendingPieces[*deal.PublishCid] = &pendingPiece{
		size:     size,
		deal:     deal,
		data:     data,
		assigned: false,
		accepted: func(sn abi.SectorNumber, offset abi.UnpaddedPieceSize, err error) {
			resCh <- struct {
				sn     abi.SectorNumber
				offset abi.UnpaddedPieceSize
				err    error
			}{sn: sn, offset: offset, err: err}
		},
	}

	go func() {
		defer m.inputLk.Unlock()
		if err := m.updateInput(ctx, sp); err != nil {
			log.Errorf("%+v", err)
		}
	}()

	res := <-resCh

	return res.sn, res.offset.Padded(), res.err
}

// called with m.inputLk
func (m *Sealing) updateInput(ctx context.Context, sp abi.RegisteredSealProof) error {
	ssize, err := sp.SectorSize()
	if err != nil {
		return err
	}

	type match struct {
		sector abi.SectorID
		deal   cid.Cid

		size    abi.UnpaddedPieceSize
		padding abi.UnpaddedPieceSize
	}

	var matches []match
	toAssign := map[cid.Cid]struct{}{} // used to maybe create new sectors

	// todo: this is distinctly O(n^2), may need to be optimized for tiny deals and large scale miners
	//  (unlikely to be a problem now)
	for pieceCid, piece := range m.pendingPieces {
		if piece.assigned {
			continue // already assigned to a sector, skip
		}

		toAssign[pieceCid] = struct{}{}

		for id, sector := range m.openSectors {
			avail := abi.PaddedPieceSize(ssize).Unpadded() - sector.used

			if piece.size <= avail { // (note: if we have enough space for the piece, we also have enough space for inter-piece padding)
				matches = append(matches, match{
					sector: id,
					deal:   pieceCid,

					size:    piece.size,
					padding: avail % piece.size,
				})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].padding != matches[j].padding { // less padding is better
			return matches[i].padding < matches[j].padding
		}

		if matches[i].size != matches[j].size { // larger pieces are better
			return matches[i].size < matches[j].size
		}

		return matches[i].sector.Number < matches[j].sector.Number // prefer older sectors
	})

	var assigned int
	for _, mt := range matches {
		if m.pendingPieces[mt.deal].assigned {
			assigned++
			continue
		}

		if _, found := m.openSectors[mt.sector]; !found {
			continue
		}

		err := m.openSectors[mt.sector].maybeAccept(mt.deal)
		if err != nil {
			m.pendingPieces[mt.deal].accepted(mt.sector.Number, 0, err) // non-error case in handleAddPiece
		}

		m.pendingPieces[mt.deal].assigned = true
		delete(toAssign, mt.deal)

		if err != nil {
			log.Errorf("sector %d rejected deal %s: %+v", mt.sector, mt.deal, err)
			continue
		}

		delete(m.openSectors, mt.sector)
	}

	if len(toAssign) > 0 {
		if err := m.tryCreateDealSector(ctx, sp); err != nil {
			log.Errorw("Failed to create a new sector for deals", "error", err)
		}
	}

	return nil
}

func (m *Sealing) tryCreateDealSector(ctx context.Context, sp abi.RegisteredSealProof) error {
	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting storage config: %w", err)
	}

	if cfg.MaxSealingSectorsForDeals > 0 && m.stats.curSealing() > cfg.MaxSealingSectorsForDeals {
		return nil
	}

	if cfg.MaxWaitDealsSectors > 0 && m.stats.curStaging() > cfg.MaxWaitDealsSectors {
		return nil
	}

	// Now actually create a new sector

	sid, err := m.sc.Next()
	if err != nil {
		return xerrors.Errorf("getting sector number: %w", err)
	}

	err = m.sealer.NewSector(ctx, m.minerSector(sp, sid))
	if err != nil {
		return xerrors.Errorf("initializing sector: %w", err)
	}

	log.Infow("Creating sector", "number", sid, "type", "deal", "proofType", sp)
	return m.sectors.Send(uint64(sid), SectorStart{
		ID:         sid,
		SectorType: sp,
	})
}

func (m *Sealing) StartPacking(sid abi.SectorNumber) error {
	return m.sectors.Send(uint64(sid), SectorStartPacking{})
}
