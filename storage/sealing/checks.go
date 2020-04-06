package sealing

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/sector-storage/zerocomm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
)

// TODO: For now we handle this by halting state execution, when we get jsonrpc reconnecting
//  We should implement some wait-for-api logic
type ErrApi struct{ error }

type ErrInvalidDeals struct{ error }
type ErrInvalidPiece struct{ error }
type ErrExpiredDeals struct{ error }

type ErrBadCommD struct{ error }
type ErrExpiredTicket struct{ error }

type ErrBadSeed struct{ error }
type ErrInvalidProof struct{ error }

// checkPieces validates that:
//  - Each piece han a corresponding on chain deal
//  - Piece commitments match with on chain deals
//  - Piece sizes match
//  - Deals aren't expired
func checkPieces(ctx context.Context, si SectorInfo, api SealingAPI) error {
	tok, height, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrApi{xerrors.Errorf("getting chain head: %w", err)}
	}

	for i, piece := range si.Pieces {
		if piece.DealID == nil {
			exp := zerocomm.ZeroPieceCommitment(piece.Size)
			if piece.CommP != exp {
				return &ErrInvalidPiece{xerrors.Errorf("deal %d piece %d had non-zero CommP %+v", piece.DealID, i, piece.CommP)}
			}
			continue
		}
		proposal, _, err := api.StateMarketStorageDeal(ctx, *piece.DealID, tok)
		if err != nil {
			return &ErrApi{xerrors.Errorf("getting deal %d for piece %d: %w", piece.DealID, i, err)}
		}

		if proposal.PieceCID != piece.CommP {
			return &ErrInvalidDeals{xerrors.Errorf("piece %d (or %d) of sector %d refers deal %d with wrong CommP: %x != %x", i, len(si.Pieces), si.SectorID, piece.DealID, piece.CommP, proposal.PieceCID)}
		}

		if piece.Size != proposal.PieceSize.Unpadded() {
			return &ErrInvalidDeals{xerrors.Errorf("piece %d (or %d) of sector %d refers deal %d with different size: %d != %d", i, len(si.Pieces), si.SectorID, piece.DealID, piece.Size, proposal.PieceSize)}
		}

		if height >= proposal.StartEpoch {
			return &ErrExpiredDeals{xerrors.Errorf("piece %d (or %d) of sector %d refers expired deal %d - should start at %d, head %d", i, len(si.Pieces), si.SectorID, piece.DealID, proposal.StartEpoch, height)}
		}
	}

	return nil
}

// checkPrecommit checks that data commitment generated in the sealing process
//  matches pieces, and that the seal ticket isn't expired
func checkPrecommit(ctx context.Context, maddr address.Address, si SectorInfo, api SealingAPI) (err error) {
	tok, height, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrApi{xerrors.Errorf("getting chain head: %w", err)}
	}

	commD, err := api.StateComputeDataCommitment(ctx, maddr, si.SectorType, si.deals(), tok)
	if err != nil {
		return &ErrApi{xerrors.Errorf("calling StateComputeDataCommitment: %w", err)}
	}

	if !commD.Equals(*si.CommD) {
		return &ErrBadCommD{xerrors.Errorf("on chain CommD differs from sector: %s != %s", commD, si.CommD)}
	}

	if int64(height)-int64(si.TicketEpoch+SealRandomnessLookback) > SealRandomnessLookbackLimit {
		return &ErrExpiredTicket{xerrors.Errorf("ticket expired: seal height: %d, head: %d", si.TicketEpoch+SealRandomnessLookback, height)}
	}

	return nil
}

func (m *Sealing) checkCommit(ctx context.Context, si SectorInfo, proof []byte) (err error) {
	tok, _, err := m.api.ChainHead(ctx)
	if err != nil {
		return &ErrApi{xerrors.Errorf("getting chain head: %w", err)}
	}

	if si.SeedEpoch == 0 {
		return &ErrBadSeed{xerrors.Errorf("seed epoch was not set")}
	}

	pci, err := m.api.StateGetSectorPreCommitOnChainInfo(ctx, m.maddr, si.SectorID, tok)
	if err != nil {
		return xerrors.Errorf("getting precommit info: %w", err)
	}

	if pci.PreCommitEpoch+miner.PreCommitChallengeDelay != si.SeedEpoch {
		return &ErrBadSeed{xerrors.Errorf("seed epoch doesn't match on chain info: %d != %d", pci.PreCommitEpoch+miner.PreCommitChallengeDelay, si.SeedEpoch)}
	}

	seed, err := m.api.ChainGetRandomness(ctx, tok, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, si.SeedEpoch, nil)
	if err != nil {
		return &ErrApi{xerrors.Errorf("failed to get randomness for computing seal proof: %w", err)}
	}

	if string(seed) != string(si.SeedValue) {
		return &ErrBadSeed{xerrors.Errorf("seed has changed")}
	}

	ss, err := m.api.StateMinerSectorSize(ctx, m.maddr, tok)
	if err != nil {
		return &ErrApi{err}
	}
	_, spt, err := ffiwrapper.ProofTypeFromSectorSize(ss)
	if err != nil {
		return err
	}

	if *si.CommR != pci.Info.SealedCID {
		log.Warn("on-chain sealed CID doesn't match!")
	}

	ok, err := m.verif.VerifySeal(abi.SealVerifyInfo{
		SectorID: m.minerSector(si.SectorID),
		OnChain: abi.OnChainSealVerifyInfo{
			SealedCID:        pci.Info.SealedCID,
			InteractiveEpoch: si.SeedEpoch,
			RegisteredProof:  spt,
			Proof:            proof,
			SectorNumber:     si.SectorID,
			SealRandEpoch:    si.TicketEpoch,
		},
		Randomness:            si.TicketValue,
		InteractiveRandomness: si.SeedValue,
		UnsealedCID:           *si.CommD,
	})
	if err != nil {
		return xerrors.Errorf("verify seal: %w", err)
	}
	if !ok {
		return &ErrInvalidProof{xerrors.New("invalid proof (compute error?)")}
	}

	return nil
}
