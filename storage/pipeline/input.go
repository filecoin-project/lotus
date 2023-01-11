package sealing

import (
	"context"
	"sort"
	"time"

	"github.com/ipfs/go-cid"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/pipeline/lib/nullreader"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

func (m *Sealing) handleWaitDeals(ctx statemachine.Context, sector SectorInfo) error {
	var used abi.UnpaddedPieceSize
	var lastDealEnd abi.ChainEpoch
	for _, piece := range sector.Pieces {
		used += piece.Piece.Size.Unpadded()

		if piece.DealInfo != nil && piece.DealInfo.DealProposal.EndEpoch > lastDealEnd {
			lastDealEnd = piece.DealInfo.DealProposal.EndEpoch
		}
	}

	m.inputLk.Lock()

	if m.nextDealSector != nil && *m.nextDealSector == sector.SectorNumber {
		m.nextDealSector = nil
	}

	sid := m.minerSectorID(sector.SectorNumber)

	if len(m.assignedPieces[sid]) > 0 {
		m.inputLk.Unlock()
		// got assigned more pieces in the AddPiece state
		return ctx.Send(SectorAddPiece{})
	}

	started, err := m.maybeStartSealing(ctx, sector, used)
	if err != nil || started {
		delete(m.openSectors, m.minerSectorID(sector.SectorNumber))

		m.inputLk.Unlock()

		return err
	}

	if _, has := m.openSectors[sid]; !has {
		m.openSectors[sid] = &openSector{
			used: used,
			maybeAccept: func(cid cid.Cid) error {
				// todo check deal start deadline (configurable)
				m.assignedPieces[sid] = append(m.assignedPieces[sid], cid)

				return ctx.Send(SectorAddPiece{})
			},
			number:   sector.SectorNumber,
			ccUpdate: sector.CCUpdate,
		}
	} else {
		// make sure we're only accounting for pieces which were correctly added
		// (note that m.assignedPieces[sid] will always be empty here)
		m.openSectors[sid].used = used
	}
	m.openSectors[sid].lastDealEnd = lastDealEnd

	go func() {
		defer m.inputLk.Unlock()
		if err := m.updateInput(ctx.Context(), sector.SectorType); err != nil {
			log.Errorf("%+v", err)
		}
	}()

	return nil
}

func (m *Sealing) maybeStartSealing(ctx statemachine.Context, sector SectorInfo, used abi.UnpaddedPieceSize) (bool, error) {
	log := log.WithOptions(zap.Fields(
		zap.Uint64("sector", uint64(sector.SectorNumber)),
		zap.Int("deals", len(sector.dealIDs())),
	))

	now := time.Now()
	st := m.sectorTimers[m.minerSectorID(sector.SectorNumber)]
	if st != nil {
		if !st.Stop() { // timer expired, SectorStartPacking was/is being sent
			// we send another SectorStartPacking in case one was sent in the handleAddPiece state
			log.Infow("starting to seal deal sector", "trigger", "wait-timeout")
			return true, ctx.Send(SectorStartPacking{})
		}
	}

	ssize, err := sector.SectorType.SectorSize()
	if err != nil {
		return false, xerrors.Errorf("getting sector size")
	}

	maxDeals, err := getDealPerSectorLimit(ssize)
	if err != nil {
		return false, xerrors.Errorf("getting per-sector deal limit: %w", err)
	}

	if len(sector.dealIDs()) >= maxDeals {
		// can't accept more deals
		log.Infow("starting to seal deal sector", "trigger", "maxdeals")
		return true, ctx.Send(SectorStartPacking{})
	}

	if used.Padded() == abi.PaddedPieceSize(ssize) {
		// sector full
		log.Infow("starting to seal deal sector", "trigger", "filled")
		return true, ctx.Send(SectorStartPacking{})
	}

	if sector.CreationTime != 0 {
		cfg, err := m.getConfig()
		if err != nil {
			return false, xerrors.Errorf("getting storage config: %w", err)
		}

		sealTime := time.Unix(sector.CreationTime, 0).Add(cfg.WaitDealsDelay)

		// check deal age, start sealing when the deal closest to starting is within slack time
		ts, err := m.Api.ChainHead(ctx.Context())
		blockTime := time.Second * time.Duration(build.BlockDelaySecs)
		if err != nil {
			return false, xerrors.Errorf("API error getting head: %w", err)
		}

		var dealSafeSealEpoch abi.ChainEpoch
		for _, piece := range sector.Pieces {
			if piece.DealInfo == nil {
				continue
			}

			dealSafeSealEpoch = piece.DealInfo.DealProposal.StartEpoch - cfg.StartEpochSealingBuffer

			alloc, _ := m.Api.StateGetAllocationForPendingDeal(ctx.Context(), piece.DealInfo.DealID, types.EmptyTSK)
			// alloc is nil if this is not a verified deal in nv17 or later
			if alloc == nil {
				continue
			}

			if alloc.Expiration-cfg.StartEpochSealingBuffer < dealSafeSealEpoch {
				dealSafeSealEpoch = alloc.Expiration - cfg.StartEpochSealingBuffer
				log.Debugw("checking safe seal epoch", "dealSafeSealEpoch", dealSafeSealEpoch)
			}
		}

		dealSafeSealTime := time.Now().Add(time.Duration(dealSafeSealEpoch-ts.Height()) * blockTime)
		if dealSafeSealTime.Before(sealTime) {
			log.Debugw("deal safe time is before seal time", "dealSafeSealTime", dealSafeSealTime, "sealTime", sealTime)
			sealTime = dealSafeSealTime
		}

		if now.After(sealTime) {
			log.Infow("starting to seal deal sector", "trigger", "wait-timeout", "creation", sector.CreationTime)
			return true, ctx.Send(SectorStartPacking{})
		}

		m.sectorTimers[m.minerSectorID(sector.SectorNumber)] = time.AfterFunc(sealTime.Sub(now), func() {
			log.Infow("starting to seal deal sector", "trigger", "wait-timer")

			if err := ctx.Send(SectorStartPacking{}); err != nil {
				log.Errorw("sending SectorStartPacking event failed", "error", err)
			}
		})
	}

	return false, nil
}

func (m *Sealing) handleAddPiece(ctx statemachine.Context, sector SectorInfo) error {
	ssize, err := sector.SectorType.SectorSize()
	if err != nil {
		return err
	}

	res := SectorPieceAdded{}

	m.inputLk.Lock()

	pending, ok := m.assignedPieces[m.minerSectorID(sector.SectorNumber)]
	if ok {
		delete(m.assignedPieces, m.minerSectorID(sector.SectorNumber))
	}
	m.inputLk.Unlock()
	if !ok {
		// nothing to do here (might happen after a restart in AddPiece)
		return ctx.Send(res)
	}

	var offset abi.UnpaddedPieceSize
	pieceSizes := make([]abi.UnpaddedPieceSize, len(sector.Pieces))
	for i, p := range sector.Pieces {
		pieceSizes[i] = p.Piece.Size.Unpadded()
		offset += p.Piece.Size.Unpadded()
	}

	maxDeals, err := getDealPerSectorLimit(ssize)
	if err != nil {
		return xerrors.Errorf("getting per-sector deal limit: %w", err)
	}

	for i, piece := range pending {
		m.inputLk.Lock()
		deal, ok := m.pendingPieces[piece]
		m.inputLk.Unlock()
		if !ok {
			return xerrors.Errorf("piece %s assigned to sector %d not found", piece, sector.SectorNumber)
		}

		if len(sector.dealIDs())+(i+1) > maxDeals {
			// todo: this is rather unlikely to happen, but in case it does, return the deal to waiting queue instead of failing it
			deal.accepted(sector.SectorNumber, offset, xerrors.Errorf("too many deals assigned to sector %d, dropping deal", sector.SectorNumber))
			continue
		}

		pads, padLength := ffiwrapper.GetRequiredPadding(offset.Padded(), deal.size.Padded())

		if offset.Padded()+padLength+deal.size.Padded() > abi.PaddedPieceSize(ssize) {
			// todo: this is rather unlikely to happen, but in case it does, return the deal to waiting queue instead of failing it
			deal.accepted(sector.SectorNumber, offset, xerrors.Errorf("piece %s assigned to sector %d with not enough space", piece, sector.SectorNumber))
			continue
		}

		offset += padLength.Unpadded()

		for _, p := range pads {
			expectCid := zerocomm.ZeroPieceCommitment(p.Unpadded())

			ppi, err := m.sealer.AddPiece(sealer.WithPriority(ctx.Context(), DealSectorPriority),
				m.minerSector(sector.SectorType, sector.SectorNumber),
				pieceSizes,
				p.Unpadded(),
				nullreader.NewNullReader(p.Unpadded()))
			if err != nil {
				err = xerrors.Errorf("writing padding piece: %w", err)
				deal.accepted(sector.SectorNumber, offset, err)
				return ctx.Send(SectorAddPieceFailed{err})
			}
			if !ppi.PieceCID.Equals(expectCid) {
				err = xerrors.Errorf("got unexpected padding piece CID: expected:%s, got:%s", expectCid, ppi.PieceCID)
				deal.accepted(sector.SectorNumber, offset, err)
				return ctx.Send(SectorAddPieceFailed{err})
			}

			pieceSizes = append(pieceSizes, p.Unpadded())
			res.NewPieces = append(res.NewPieces, api.SectorPiece{
				Piece: ppi,
			})
		}

		ppi, err := m.sealer.AddPiece(sealer.WithPriority(ctx.Context(), DealSectorPriority),
			m.minerSector(sector.SectorType, sector.SectorNumber),
			pieceSizes,
			deal.size,
			deal.data)
		if err != nil {
			err = xerrors.Errorf("writing piece: %w", err)
			deal.accepted(sector.SectorNumber, offset, err)
			return ctx.Send(SectorAddPieceFailed{err})
		}
		if !ppi.PieceCID.Equals(deal.deal.DealProposal.PieceCID) {
			err = xerrors.Errorf("got unexpected piece CID: expected:%s, got:%s", deal.deal.DealProposal.PieceCID, ppi.PieceCID)
			deal.accepted(sector.SectorNumber, offset, err)
			return ctx.Send(SectorAddPieceFailed{err})
		}

		log.Infow("deal added to a sector", "deal", deal.deal.DealID, "sector", sector.SectorNumber, "piece", ppi.PieceCID)

		deal.accepted(sector.SectorNumber, offset, nil)

		offset += deal.size
		pieceSizes = append(pieceSizes, deal.size)

		res.NewPieces = append(res.NewPieces, api.SectorPiece{
			Piece:    ppi,
			DealInfo: &deal.deal,
		})
	}

	return ctx.Send(res)
}

func (m *Sealing) handleAddPieceFailed(ctx statemachine.Context, sector SectorInfo) error {
	return ctx.Send(SectorRetryWaitDeals{})
}

func (m *Sealing) SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, data storiface.Data, deal api.PieceDealInfo) (api.SectorOffset, error) {
	log.Infof("Adding piece for deal %d (publish msg: %s)", deal.DealID, deal.PublishCid)
	if (padreader.PaddedSize(uint64(size))) != size {
		return api.SectorOffset{}, xerrors.Errorf("cannot allocate unpadded piece")
	}

	sp, err := m.currentSealProof(ctx)
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("getting current seal proof type: %w", err)
	}

	ssize, err := sp.SectorSize()
	if err != nil {
		return api.SectorOffset{}, err
	}

	if size > abi.PaddedPieceSize(ssize).Unpadded() {
		return api.SectorOffset{}, xerrors.Errorf("piece cannot fit into a sector")
	}

	if _, err := deal.DealProposal.Cid(); err != nil {
		return api.SectorOffset{}, xerrors.Errorf("getting proposal CID: %w", err)
	}

	cfg, err := m.getConfig()
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("getting config: %w", err)
	}

	ts, err := m.Api.ChainHead(ctx)
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("couldnt get chain head: %w", err)
	}
	if ts.Height()+cfg.StartEpochSealingBuffer > deal.DealProposal.StartEpoch {
		return api.SectorOffset{}, xerrors.Errorf(
			"cannot add piece for deal with piece CID %s: current epoch %d has passed deal proposal start epoch %d",
			deal.DealProposal.PieceCID, ts.Height(), deal.DealProposal.StartEpoch)
	}

	claimTerms, err := m.getClaimTerms(ctx, deal, ts.Key())
	if err != nil {
		return api.SectorOffset{}, err
	}

	m.inputLk.Lock()
	if pp, exist := m.pendingPieces[proposalCID(deal)]; exist {
		m.inputLk.Unlock()

		// we already have a pre-existing add piece call for this deal, let's wait for it to finish and see if it's successful
		res, err := waitAddPieceResp(ctx, pp)
		if err != nil {
			return api.SectorOffset{}, err
		}
		if res.err == nil {
			// all good, return the response
			return api.SectorOffset{Sector: res.sn, Offset: res.offset.Padded()}, res.err
		}
		// if there was an error waiting for a pre-existing add piece call, let's retry
		m.inputLk.Lock()
	}

	// addPendingPiece takes over m.inputLk
	pp := m.addPendingPiece(ctx, size, data, deal, claimTerms, sp)

	res, err := waitAddPieceResp(ctx, pp)
	if err != nil {
		return api.SectorOffset{}, err
	}
	return api.SectorOffset{Sector: res.sn, Offset: res.offset.Padded()}, res.err
}

func (m *Sealing) getClaimTerms(ctx context.Context, deal api.PieceDealInfo, tsk types.TipSetKey) (pieceClaimBounds, error) {
	nv, err := m.Api.StateNetworkVersion(ctx, tsk)
	if err != nil {
		return pieceClaimBounds{}, err
	}

	if nv >= network.Version17 {
		all, err := m.Api.StateGetAllocationForPendingDeal(ctx, deal.DealID, tsk)
		if err != nil {
			return pieceClaimBounds{}, err
		}
		if all != nil {
			return pieceClaimBounds{
				claimTermEnd: deal.DealProposal.StartEpoch + all.TermMax,
			}, nil
		}
	}

	// no allocation for this deal, so just use a really high number for "term end"
	return pieceClaimBounds{
		claimTermEnd: deal.DealProposal.EndEpoch + policy.GetSectorMaxLifetime(abi.RegisteredSealProof_StackedDrg32GiBV1_1, network.Version17),
	}, nil
}

// called with m.inputLk; transfers the lock to another goroutine!
func (m *Sealing) addPendingPiece(ctx context.Context, size abi.UnpaddedPieceSize, data storiface.Data, deal api.PieceDealInfo, ct pieceClaimBounds, sp abi.RegisteredSealProof) *pendingPiece {
	doneCh := make(chan struct{})
	pp := &pendingPiece{
		size:       size,
		deal:       deal,
		claimTerms: ct,

		data: data,

		doneCh:   doneCh,
		assigned: false,
	}
	pp.accepted = func(sn abi.SectorNumber, offset abi.UnpaddedPieceSize, err error) {
		pp.resp = &pieceAcceptResp{sn, offset, err}
		close(pp.doneCh)
	}

	m.pendingPieces[proposalCID(deal)] = pp
	go func() {
		defer m.inputLk.Unlock()
		if err := m.updateInput(ctx, sp); err != nil {
			log.Errorf("%+v", err)
		}
	}()

	return pp
}

func waitAddPieceResp(ctx context.Context, pp *pendingPiece) (*pieceAcceptResp, error) {
	select {
	case <-pp.doneCh:
		res := pp.resp
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *Sealing) SectorMatchPendingPiecesToOpenSectors(ctx context.Context) error {
	sp, err := m.currentSealProof(ctx)
	if err != nil {
		return xerrors.Errorf("failed to get current seal proof: %w", err)
	}
	log.Debug("pieces to sector matching waiting for lock")
	m.inputLk.Lock()
	defer m.inputLk.Unlock()
	return m.updateInput(ctx, sp)
}

type expFn func(sn abi.SectorNumber) (abi.ChainEpoch, abi.TokenAmount, error)

// called with m.inputLk
func (m *Sealing) updateInput(ctx context.Context, sp abi.RegisteredSealProof) error {
	memo := make(map[abi.SectorNumber]struct {
		e abi.ChainEpoch
		p abi.TokenAmount
	})
	getExpirationCached := func(sn abi.SectorNumber) (abi.ChainEpoch, abi.TokenAmount, error) {
		if e, ok := memo[sn]; ok {
			return e.e, e.p, nil
		}
		onChainInfo, err := m.Api.StateSectorGetInfo(ctx, m.maddr, sn, types.TipSetKey{})
		if err != nil {
			return 0, big.Zero(), err
		}
		if onChainInfo == nil {
			return 0, big.Zero(), xerrors.Errorf("sector info for sector %d not found", sn)
		}
		memo[sn] = struct {
			e abi.ChainEpoch
			p abi.TokenAmount
		}{e: onChainInfo.Expiration, p: onChainInfo.InitialPledge}
		return onChainInfo.Expiration, onChainInfo.InitialPledge, nil
	}

	ssize, err := sp.SectorSize()
	if err != nil {
		return err
	}

	type match struct {
		sector abi.SectorID
		deal   cid.Cid

		dealEnd      abi.ChainEpoch
		claimTermEnd abi.ChainEpoch

		size    abi.UnpaddedPieceSize
		padding abi.UnpaddedPieceSize
	}

	var matches []match
	toAssign := map[cid.Cid]struct{}{} // used to maybe create new sectors

	// todo: this is distinctly O(n^2), may need to be optimized for tiny deals and large scale miners
	//  (unlikely to be a problem now)
	for proposalCid, piece := range m.pendingPieces {
		if piece.assigned {
			continue // already assigned to a sector, skip
		}

		toAssign[proposalCid] = struct{}{}

		for id, sector := range m.openSectors {
			avail := abi.PaddedPieceSize(ssize).Unpadded() - sector.used
			// check that sector lifetime is long enough to fit deal using latest expiration from on chain

			ok, err := sector.checkDealAssignable(piece, getExpirationCached)
			if err != nil {
				log.Errorf("failed to check expiration for cc Update sector %d", sector.number)
				continue
			}
			if !ok {
				continue
			}

			if piece.size <= avail { // (note: if we have enough space for the piece, we also have enough space for inter-piece padding)
				matches = append(matches, match{
					sector: id,
					deal:   proposalCid,

					dealEnd:      piece.deal.DealProposal.EndEpoch,
					claimTermEnd: piece.claimTerms.claimTermEnd,

					size:    piece.size,
					padding: avail % piece.size,
				})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		// todo maybe sort by expiration

		if matches[i].padding != matches[j].padding { // less padding is better
			return matches[i].padding < matches[j].padding
		}

		if matches[i].size != matches[j].size { // larger pieces are better
			return matches[i].size < matches[j].size
		}

		return matches[i].sector.Number < matches[j].sector.Number // prefer older sectors
	})

	log.Debugw("updateInput matching", "matches", len(matches), "toAssign", len(toAssign), "openSectors", len(m.openSectors), "pieces", len(m.pendingPieces))

	var assigned int
	for _, mt := range matches {
		if m.pendingPieces[mt.deal].assigned {
			assigned++
			continue
		}

		if _, found := m.openSectors[mt.sector]; !found {
			continue
		}

		// late checks

		avail := abi.PaddedPieceSize(ssize).Unpadded() - m.openSectors[mt.sector].used

		if mt.size > avail {
			continue
		}

		if m.openSectors[mt.sector].lastDealEnd > mt.claimTermEnd {
			continue
		}

		// assign the piece!

		err := m.openSectors[mt.sector].maybeAccept(mt.deal)
		if err != nil {
			m.pendingPieces[mt.deal].accepted(mt.sector.Number, 0, err) // non-error case in handleAddPiece
		}

		m.openSectors[mt.sector].used += mt.padding + mt.size
		if mt.dealEnd > m.openSectors[mt.sector].lastDealEnd {
			m.openSectors[mt.sector].lastDealEnd = mt.dealEnd
		}

		m.pendingPieces[mt.deal].assigned = true
		delete(toAssign, mt.deal)

		if err != nil {
			log.Errorf("sector %d rejected deal %s: %+v", mt.sector, mt.deal, err)
			continue
		}
	}

	log.Debugw("updateInput matching done", "matches", len(matches), "toAssign", len(toAssign), "assigned", assigned, "openSectors", len(m.openSectors), "pieces", len(m.pendingPieces))

	if len(toAssign) > 0 {
		log.Errorf("we are trying to create a new sector with open sectors %v", m.openSectors)
		if err := m.tryGetDealSector(ctx, sp, getExpirationCached); err != nil {
			log.Errorw("Failed to create a new sector for deals", "error", err)
		}
	}

	return nil
}

// pendingPieceIndex is an index in the Sealing.pendingPieces map
type pendingPieceIndex cid.Cid

type pieceBound struct {
	epoch abi.ChainEpoch

	// boundStart marks deal /end/ epoch; only deals with boundStart lower or equal to expiration of a given sector can be
	// put into that sector
	boundStart []pendingPieceIndex

	// boundEnd marks deal claim TermMax; only deals with boundEnd higher or equal to expiration of a given sector can be
	// put into that sector
	boundEnd []pendingPieceIndex

	dealBytesInBound abi.UnpaddedPieceSize
}

func (m *Sealing) pendingPieceEpochBounds() []pieceBound {
	boundsByEpoch := map[abi.ChainEpoch]*pieceBound{}

	for ppi, piece := range m.pendingPieces {
		// start bound on deal end
		if boundsByEpoch[piece.deal.DealProposal.EndEpoch] == nil {
			boundsByEpoch[piece.deal.DealProposal.EndEpoch] = &pieceBound{
				epoch: piece.deal.DealProposal.EndEpoch,
			}
		}
		boundsByEpoch[piece.deal.DealProposal.EndEpoch].boundStart = append(boundsByEpoch[piece.deal.DealProposal.EndEpoch].boundStart, pendingPieceIndex(ppi))

		// end bound on term max
		if boundsByEpoch[piece.claimTerms.claimTermEnd] == nil {
			boundsByEpoch[piece.claimTerms.claimTermEnd] = &pieceBound{
				epoch: piece.claimTerms.claimTermEnd,
			}
		}
		boundsByEpoch[piece.claimTerms.claimTermEnd].boundEnd = append(boundsByEpoch[piece.claimTerms.claimTermEnd].boundEnd, pendingPieceIndex(ppi))
	}

	out := make([]pieceBound, 0, len(boundsByEpoch))
	for _, bound := range boundsByEpoch {
		out = append(out, *bound)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].epoch < out[j].epoch
	})

	var curBoundBytes abi.UnpaddedPieceSize
	for i, bound := range out {
		for _, ppi := range bound.boundStart {
			curBoundBytes += m.pendingPieces[cid.Cid(ppi)].size
		}
		for _, ppi := range bound.boundEnd {
			curBoundBytes -= m.pendingPieces[cid.Cid(ppi)].size
		}

		out[i].dealBytesInBound = curBoundBytes
	}

	return out
}

func (m *Sealing) maybeUpgradeSector(ctx context.Context, sp abi.RegisteredSealProof, cfg sealiface.Config, ef expFn) (bool, error) {
	if len(m.available) == 0 {
		return false, nil
	}

	ts, err := m.Api.ChainHead(ctx)
	if err != nil {
		return false, err
	}
	ssize, err := sp.SectorSize()
	if err != nil {
		return false, err
	}

	pieceBounds := m.pendingPieceEpochBounds()

	findBound := func(sectorExp abi.ChainEpoch) *pieceBound {
		if len(pieceBounds) == 0 {
			return nil
		}
		f := sort.Search(len(pieceBounds), func(i int) bool {
			return sectorExp <= pieceBounds[i].epoch
		})
		if f == 0 {
			// all piece bounds are after sector expiration
			return nil
		}
		return &pieceBounds[f-1]
	}

	targetExpirationEpoch := ts.Height() + abi.ChainEpoch(cfg.MinTargetUpgradeSectorExpiration)
	minExpirationEpoch := ts.Height() + abi.ChainEpoch(cfg.MinUpgradeSectorExpiration)

	var candidate abi.SectorID
	var bestExpiration abi.ChainEpoch
	var bestDealBytes abi.PaddedPieceSize
	bestPledge := types.TotalFilecoinInt

	for s := range m.available {
		expirationEpoch, pledge, err := ef(s.Number)
		if err != nil {
			log.Errorw("checking sector expiration", "error", err)
			continue
		}

		slowChecks := func(sid abi.SectorNumber) bool {
			active, err := m.sectorActive(ctx, types.TipSetKey{}, sid)
			if err != nil {
				log.Errorw("checking sector active", "error", err)
				return false
			}
			if !active {
				log.Debugw("skipping available sector", "sector", sid, "reason", "not active")
				return false
			}
			return true
		}

		if expirationEpoch < minExpirationEpoch {
			log.Debugw("skipping available sector", "sector", s.Number, "reason", "expiration below MinUpgradeSectorExpiration")
		}

		pb := findBound(expirationEpoch)
		if pb == nil {
			log.Debugw("skipping available sector", "sector", s.Number, "reason", "expiration below deal bounds")
			continue
		}

		// if the sector has less than one sector worth of candidate deals, and
		// the best candidate has more candidate deals, this sector isn't better
		if pb.dealBytesInBound.Padded() < abi.PaddedPieceSize(ssize) {
			if bestDealBytes > pb.dealBytesInBound.Padded() {
				continue
			}
		}

		// if best is below target, we want larger expirations
		// if best is above target, we want lower pledge, but only if still above target

		// todo: after nv17 "target expiration" doesn't really make that much sense
		//  (tho to be fair it doesn't make too much sense now either)
		//  we probably want the lowest expiration that's still above the configured
		//  minimum, and can fit most candidate deals

		if bestExpiration < targetExpirationEpoch {
			if expirationEpoch > bestExpiration && slowChecks(s.Number) {
				bestExpiration = expirationEpoch
				bestPledge = pledge
				bestDealBytes = pb.dealBytesInBound.Padded()
				candidate = s
			}
			continue
		}

		if expirationEpoch >= targetExpirationEpoch && pledge.LessThan(bestPledge) && slowChecks(s.Number) {
			bestExpiration = expirationEpoch
			bestPledge = pledge
			bestDealBytes = pb.dealBytesInBound.Padded()
			candidate = s
		}
	}

	if bestExpiration < minExpirationEpoch {
		log.Infow("Not upgrading any sectors", "available", len(m.available), "pieces", len(m.pendingPieces), "bestExp", bestExpiration, "target", targetExpirationEpoch, "min", minExpirationEpoch, "candidate", candidate)
		// didn't find a good sector / no sectors were available
		return false, nil
	}

	log.Infow("Upgrading sector", "number", candidate.Number, "type", "deal", "proofType", sp, "expiration", bestExpiration, "pledge", types.FIL(bestPledge))
	delete(m.available, candidate)
	m.nextDealSector = &candidate.Number
	return true, m.sectors.Send(uint64(candidate.Number), SectorStartCCUpdate{})
}

// call with m.inputLk
func (m *Sealing) createSector(ctx context.Context, cfg sealiface.Config, sp abi.RegisteredSealProof) (abi.SectorNumber, error) {
	sid, err := m.NextSectorNumber(ctx)
	if err != nil {
		return 0, xerrors.Errorf("getting sector number: %w", err)
	}

	err = m.sealer.NewSector(ctx, m.minerSector(sp, sid))
	if err != nil {
		return 0, xerrors.Errorf("initializing sector: %w", err)
	}

	// update stats early, fsm planner would do that async
	m.stats.updateSector(ctx, cfg, m.minerSectorID(sid), UndefinedSectorState)

	return sid, err
}

func (m *Sealing) tryGetDealSector(ctx context.Context, sp abi.RegisteredSealProof, ef expFn) error {
	m.startupWait.Wait()

	if m.nextDealSector != nil {
		return nil // new sector is being created right now
	}

	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting storage config: %w", err)
	}

	// if we're above WaitDeals limit, we don't want to add more staging sectors
	if cfg.MaxWaitDealsSectors > 0 && m.stats.curStaging() >= cfg.MaxWaitDealsSectors {
		return nil
	}

	maxUpgrading := cfg.MaxSealingSectorsForDeals
	if cfg.MaxUpgradingSectors > 0 {
		maxUpgrading = cfg.MaxUpgradingSectors
	}

	canCreate := cfg.MakeNewSectorForDeals && !(cfg.MaxSealingSectorsForDeals > 0 && m.stats.curSealing() >= cfg.MaxSealingSectorsForDeals)
	canUpgrade := !(maxUpgrading > 0 && m.stats.curSealing() >= maxUpgrading)

	// we want to try to upgrade when:
	// - we can upgrade and prefer upgrades
	// - we don't prefer upgrades, but can't create a new sector
	shouldUpgrade := canUpgrade && (!cfg.PreferNewSectorsForDeals || !canCreate)

	log.Infow("new deal sector decision",
		"sealing", m.stats.curSealing(),
		"maxSeal", cfg.MaxSealingSectorsForDeals,
		"maxUpgrade", maxUpgrading,
		"preferNew", cfg.PreferNewSectorsForDeals,
		"canCreate", canCreate,
		"canUpgrade", canUpgrade,
		"shouldUpgrade", shouldUpgrade)

	if shouldUpgrade {
		got, err := m.maybeUpgradeSector(ctx, sp, cfg, ef)
		if err != nil {
			return err
		}
		if got {
			return nil
		}
	}

	if canCreate {
		sid, err := m.createSector(ctx, cfg, sp)
		if err != nil {
			return err
		}
		m.nextDealSector = &sid

		log.Infow("Creating sector", "number", sid, "type", "deal", "proofType", sp)
		if err := m.sectors.Send(uint64(sid), SectorStart{
			ID:         sid,
			SectorType: sp,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Sealing) StartPackingSector(sid abi.SectorNumber) error {
	m.startupWait.Wait()

	log.Infow("starting to seal deal sector", "sector", sid, "trigger", "user")
	return m.sectors.Send(uint64(sid), SectorStartPacking{})
}

func (m *Sealing) SectorAbortUpgrade(sid abi.SectorNumber) error {
	m.startupWait.Wait()

	m.inputLk.Lock()
	// always do this early
	delete(m.available, m.minerSectorID(sid))
	m.inputLk.Unlock()

	log.Infow("aborting upgrade of sector", "sector", sid, "trigger", "user")
	return m.sectors.Send(uint64(sid), SectorAbortUpgrade{xerrors.New("triggered by user")})
}

func (m *Sealing) SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) {
	if showOnChainInfo {
		return api.SectorInfo{}, xerrors.Errorf("on-chain info not supported")
	}

	info, err := m.GetSectorInfo(sid)
	if err != nil {
		return api.SectorInfo{}, err
	}

	deals := make([]abi.DealID, len(info.Pieces))
	pieces := make([]api.SectorPiece, len(info.Pieces))
	for i, piece := range info.Pieces {
		pieces[i].Piece = piece.Piece
		if piece.DealInfo == nil {
			continue
		}

		pdi := *piece.DealInfo // copy
		pieces[i].DealInfo = &pdi

		deals[i] = piece.DealInfo.DealID
	}

	log := make([]api.SectorLog, len(info.Log))
	for i, l := range info.Log {
		log[i] = api.SectorLog{
			Kind:      l.Kind,
			Timestamp: l.Timestamp,
			Trace:     l.Trace,
			Message:   l.Message,
		}
	}

	sInfo := api.SectorInfo{
		SectorID: sid,
		State:    api.SectorState(info.State),
		CommD:    info.CommD,
		CommR:    info.CommR,
		Proof:    info.Proof,
		Deals:    deals,
		Pieces:   pieces,
		Ticket: api.SealTicket{
			Value: info.TicketValue,
			Epoch: info.TicketEpoch,
		},
		Seed: api.SealSeed{
			Value: info.SeedValue,
			Epoch: info.SeedEpoch,
		},
		PreCommitMsg:         info.PreCommitMessage,
		CommitMsg:            info.CommitMessage,
		Retries:              info.InvalidProofs,
		ToUpgrade:            false,
		ReplicaUpdateMessage: info.ReplicaUpdateMessage,

		LastErr: info.LastErr,
		Log:     log,
		// on chain info
		SealProof:          info.SectorType,
		Activation:         0,
		Expiration:         0,
		DealWeight:         big.Zero(),
		VerifiedDealWeight: big.Zero(),
		InitialPledge:      big.Zero(),
		OnTime:             0,
		Early:              0,
	}

	return sInfo, nil
}

func proposalCID(deal api.PieceDealInfo) cid.Cid {
	pc, err := deal.DealProposal.Cid()
	if err != nil {
		log.Errorf("DealProposal.Cid error: %+v", err)
		return cid.Undef
	}

	return pc
}

var _ sectorblocks.SectorBuilder = &Sealing{}
