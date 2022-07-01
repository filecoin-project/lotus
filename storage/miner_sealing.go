package storage

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	pipeline "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

// TODO: refactor this to be direct somehow

func (m *Miner) Address() address.Address {
	return m.sealing.Address()
}

func (m *Miner) StartPackingSector(sectorNum abi.SectorNumber) error {
	return m.sealing.StartPacking(sectorNum)
}

func (m *Miner) ListSectors() ([]pipeline.SectorInfo, error) {
	return m.sealing.ListSectors()
}

func (m *Miner) PledgeSector(ctx context.Context) (storiface.SectorRef, error) {
	return m.sealing.PledgeSector(ctx)
}

func (m *Miner) ForceSectorState(ctx context.Context, id abi.SectorNumber, state pipeline.SectorState) error {
	return m.sealing.ForceSectorState(ctx, id, state)
}

func (m *Miner) RemoveSector(ctx context.Context, id abi.SectorNumber) error {
	return m.sealing.Remove(ctx, id)
}

func (m *Miner) TerminateSector(ctx context.Context, id abi.SectorNumber) error {
	return m.sealing.Terminate(ctx, id)
}

func (m *Miner) TerminateFlush(ctx context.Context) (*cid.Cid, error) {
	return m.sealing.TerminateFlush(ctx)
}

func (m *Miner) TerminatePending(ctx context.Context) ([]abi.SectorID, error) {
	return m.sealing.TerminatePending(ctx)
}

func (m *Miner) SectorPreCommitFlush(ctx context.Context) ([]sealiface.PreCommitBatchRes, error) {
	return m.sealing.SectorPreCommitFlush(ctx)
}

func (m *Miner) SectorPreCommitPending(ctx context.Context) ([]abi.SectorID, error) {
	return m.sealing.SectorPreCommitPending(ctx)
}

func (m *Miner) CommitFlush(ctx context.Context) ([]sealiface.CommitBatchRes, error) {
	return m.sealing.CommitFlush(ctx)
}

func (m *Miner) CommitPending(ctx context.Context) ([]abi.SectorID, error) {
	return m.sealing.CommitPending(ctx)
}

func (m *Miner) SectorMatchPendingPiecesToOpenSectors(ctx context.Context) error {
	return m.sealing.MatchPendingPiecesToOpenSectors(ctx)
}

func (m *Miner) MarkForUpgrade(ctx context.Context, id abi.SectorNumber, snap bool) error {
	if snap {
		return m.sealing.MarkForSnapUpgrade(ctx, id)
	}
	return xerrors.Errorf("Old CC upgrade deprecated, use snap deals CC upgrade")
}

func (m *Miner) SectorAbortUpgrade(sectorNum abi.SectorNumber) error {
	return m.sealing.AbortUpgrade(sectorNum)
}

func (m *Miner) SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, r storiface.Data, d api.PieceDealInfo) (api.SectorOffset, error) {
	return m.sealing.SectorAddPieceToAny(ctx, size, r, d)
}

func (m *Miner) SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) {
	if showOnChainInfo {
		return api.SectorInfo{}, xerrors.Errorf("on-chain info not supported")
	}

	info, err := m.sealing.GetSectorInfo(sid)
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

var _ sectorblocks.SectorBuilder = &Miner{}
