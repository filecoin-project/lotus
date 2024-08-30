package sealing

import (
	"bytes"
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/v2/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/proofs"
	"github.com/filecoin-project/lotus/chain/types"
)

// TODO: For now we handle this by halting state execution, when we get jsonrpc reconnecting
//
//	We should implement some wait-for-api logic

type ErrApi struct{ error }

type ErrNoDeals struct{ error }
type ErrInvalidDeals struct{ error }
type ErrInvalidPiece struct{ error }
type ErrExpiredDeals struct{ error }

type ErrBadCommD struct{ error }
type ErrExpiredTicket struct{ error }
type ErrBadTicket struct{ error }
type ErrPrecommitOnChain struct{ error }
type ErrSectorNumberAllocated struct{ error }

type ErrBadSeed struct{ error }
type ErrInvalidProof struct{ error }
type ErrNoPrecommit struct{ error }
type ErrCommitWaitFailed struct{ error }

type ErrBadRU struct{ error }
type ErrBadPR struct{ error }

func checkPieces(ctx context.Context, maddr address.Address, sn abi.SectorNumber, pieces []SafeSectorPiece, api SealingAPI, mustHaveDeals bool) error {
	ts, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrApi{xerrors.Errorf("getting chain head: %w", err)}
	}

	dealCount := 0
	var offset abi.PaddedPieceSize

	for i, p := range pieces {
		p, i := p, i

		// check that the piece is correctly aligned
		if offset%p.Piece().Size != 0 {
			return &ErrInvalidPiece{xerrors.Errorf("sector %d piece %d is not aligned: size=%xh offset=%xh off-by=%xh", sn, i, p.Piece().Size, offset, offset%p.Piece().Size)}
		}
		offset += p.Piece().Size

		err := p.handleDealInfo(handleDealInfoParams{
			FillerHandler: func(pi UniversalPieceInfo) error {
				// if no deal is associated with the piece, ensure that we added it as
				// filler (i.e. ensure that it has a zero PieceCID)

				exp := zerocomm.ZeroPieceCommitment(p.Piece().Size.Unpadded())
				if !p.Piece().PieceCID.Equals(exp) {
					return &ErrInvalidPiece{xerrors.Errorf("sector %d piece %d had non-zero PieceCID %+v", sn, i, p.Piece().PieceCID)}
				}

				return nil
			},
			BuiltinMarketHandler: func(pi UniversalPieceInfo) error {
				dealCount++

				deal, err := api.StateMarketStorageDeal(ctx, p.Impl().DealID, ts.Key())
				if err != nil {
					return &ErrInvalidDeals{xerrors.Errorf("getting deal %d for piece %d: %w", p.Impl().DealID, i, err)}
				}

				if deal.Proposal.Provider != maddr {
					return &ErrInvalidDeals{xerrors.Errorf("piece %d (of %d) of sector %d refers deal %d with wrong provider: %s != %s", i, len(pieces), sn, p.Impl().DealID, deal.Proposal.Provider, maddr)}
				}

				if deal.Proposal.PieceCID != p.Piece().PieceCID {
					return &ErrInvalidDeals{xerrors.Errorf("piece %d (of %d) of sector %d refers deal %d with wrong PieceCID: %s != %s", i, len(pieces), sn, p.Impl().DealID, p.Impl().DealProposal.PieceCID, deal.Proposal.PieceCID)}
				}

				if p.Piece().Size != deal.Proposal.PieceSize {
					return &ErrInvalidDeals{xerrors.Errorf("piece %d (of %d) of sector %d refers deal %d with different size: %d != %d", i, len(pieces), sn, p.Impl().DealID, p.Piece().Size, deal.Proposal.PieceSize)}
				}

				if ts.Height() >= deal.Proposal.StartEpoch {
					return &ErrExpiredDeals{xerrors.Errorf("piece %d (of %d) of sector %d refers expired deal %d - should start at %d, head %d", i, len(pieces), sn, p.Impl().DealID, deal.Proposal.StartEpoch, ts.Height())}
				}

				return nil
			},
			DDOHandler: func(pi UniversalPieceInfo) error {
				dealCount++

				// try to get allocation to see if that still works
				all, err := pi.GetAllocation(ctx, api, ts.Key())
				if err != nil {
					return xerrors.Errorf("getting deal %d allocation: %w", p.Impl().DealID, err)
				}
				if all != nil {
					mid, err := address.IDFromAddress(maddr)
					if err != nil {
						return xerrors.Errorf("getting miner id: %w", err)
					}

					if all.Provider != abi.ActorID(mid) {
						return xerrors.Errorf("allocation provider doesn't match miner")
					}

					if ts.Height() >= all.Expiration {
						return &ErrExpiredDeals{xerrors.Errorf("piece allocation %d (of %d) of sector %d refers expired deal %d - should start at %d, head %d", i, len(pieces), sn, p.Impl().DealID, all.Expiration, ts.Height())}
					}

					if all.Size < p.Piece().Size {
						return &ErrInvalidDeals{xerrors.Errorf("piece allocation %d (of %d) of sector %d refers deal %d with different size: %d != %d", i, len(pieces), sn, p.Impl().DealID, p.Piece().Size, all.Size)}
					}
				}

				return nil
			},
		})
		if err != nil {
			return err
		}
	}

	if mustHaveDeals && dealCount <= 0 {
		return &ErrNoDeals{xerrors.Errorf("sector %d must have deals, but does not", sn)}
	}

	return nil
}

// checkPrecommit checks that data commitment generated in the sealing process
//
//	matches pieces, and that the seal ticket isn't expired
func checkPrecommit(ctx context.Context, maddr address.Address, si SectorInfo, tsk types.TipSetKey, height abi.ChainEpoch, api SealingAPI) (err error) {
	if err := checkPieces(ctx, maddr, si.SectorNumber, si.Pieces, api, false); err != nil {
		return err
	}

	if si.hasData() {
		commD, err := computeUnsealedCIDFromPieces(si)
		if err != nil {
			return &ErrApi{xerrors.Errorf("calling StateComputeDataCommitment: %w", err)}
		}

		if si.CommD == nil || !commD.Equals(*si.CommD) {
			return &ErrBadCommD{xerrors.Errorf("on chain CommD differs from sector: %s != %s", commD, si.CommD)}
		}
	}

	pci, err := api.StateSectorPreCommitInfo(ctx, maddr, si.SectorNumber, tsk)
	if err != nil {
		return &ErrApi{xerrors.Errorf("getting precommit info: %w", err)}
	}

	if pci != nil {
		// committed P2 message
		if pci.Info.SealRandEpoch != si.TicketEpoch {
			return &ErrBadTicket{xerrors.Errorf("bad ticket epoch: %d != %d", pci.Info.SealRandEpoch, si.TicketEpoch)}
		}
		return &ErrPrecommitOnChain{xerrors.Errorf("precommit already on chain")}
	}

	alloc, err := api.StateMinerSectorAllocated(ctx, maddr, si.SectorNumber, tsk)
	if err != nil {
		return xerrors.Errorf("checking if sector is allocated: %w", err)
	}
	if alloc {
		//committed P2 message  but commit C2 message too late, pci should be null in this case
		return &ErrSectorNumberAllocated{xerrors.Errorf("sector %d is allocated, but PreCommit info wasn't found on chain", si.SectorNumber)}
	}

	//never commit P2 message before, check ticket expiration
	ticketEarliest := height - policy.MaxPreCommitRandomnessLookback

	if si.TicketEpoch < ticketEarliest {
		return &ErrExpiredTicket{xerrors.Errorf("ticket expired: seal height: %d, head: %d", si.TicketEpoch+policy.SealRandomnessLookback, height)}
	}
	return nil
}

func (m *Sealing) checkCommit(ctx context.Context, si SectorInfo, proof []byte, tsk types.TipSetKey) (err error) {
	if si.SeedEpoch == 0 {
		return &ErrBadSeed{xerrors.Errorf("seed epoch was not set")}
	}

	pci, err := m.Api.StateSectorPreCommitInfo(ctx, m.maddr, si.SectorNumber, tsk)
	if err != nil {
		return xerrors.Errorf("getting precommit info: %w", err)
	}

	if pci == nil {
		alloc, err := m.Api.StateMinerSectorAllocated(ctx, m.maddr, si.SectorNumber, tsk)
		if err != nil {
			return xerrors.Errorf("checking if sector is allocated: %w", err)
		}
		if alloc {
			// not much more we can check here, basically try to wait for commit,
			// and hope that this will work

			if si.CommitMessage != nil {
				return &ErrCommitWaitFailed{err}
			}

			return xerrors.Errorf("sector %d is allocated, but PreCommit info wasn't found on chain", si.SectorNumber)
		}

		return &ErrNoPrecommit{xerrors.Errorf("precommit info not found on-chain")}
	}

	if pci.PreCommitEpoch+policy.GetPreCommitChallengeDelay() != si.SeedEpoch {
		return &ErrBadSeed{xerrors.Errorf("seed epoch doesn't match on chain info: %d != %d", pci.PreCommitEpoch+policy.GetPreCommitChallengeDelay(), si.SeedEpoch)}
	}

	buf := new(bytes.Buffer)
	if err := m.maddr.MarshalCBOR(buf); err != nil {
		return err
	}

	seed, err := m.Api.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, si.SeedEpoch, buf.Bytes(), tsk)
	if err != nil {
		return &ErrApi{xerrors.Errorf("failed to get randomness for computing seal proof: %w", err)}
	}

	if string(seed) != string(si.SeedValue) {
		return &ErrBadSeed{xerrors.Errorf("seed has changed")}
	}

	if *si.CommR != pci.Info.SealedCID {
		log.Warn("on-chain sealed CID doesn't match!")
	}

	ok, err := m.verif.VerifySeal(prooftypes.SealVerifyInfo{
		SectorID:              m.minerSectorID(si.SectorNumber),
		SealedCID:             pci.Info.SealedCID,
		SealProof:             pci.Info.SealProof,
		Proof:                 proof,
		Randomness:            si.TicketValue,
		InteractiveRandomness: si.SeedValue,
		UnsealedCID:           *si.CommD,
	})
	if err != nil {
		return &ErrInvalidProof{xerrors.Errorf("verify seal: %w", err)}
	}
	if !ok {
		return &ErrInvalidProof{xerrors.New("invalid proof (compute error?)")}
	}

	if err := checkPieces(ctx, m.maddr, si.SectorNumber, si.Pieces, m.Api, false); err != nil {
		return err
	}

	return nil
}

// check that sector info is good after running a replica update
func checkReplicaUpdate(ctx context.Context, maddr address.Address, si SectorInfo, api SealingAPI) error {
	if err := checkPieces(ctx, maddr, si.SectorNumber, si.Pieces, api, true); err != nil {
		return err
	}
	if !si.CCUpdate {
		return xerrors.Errorf("replica update on sector not marked for update")
	}

	commD, err := computeUnsealedCIDFromPieces(si)
	if err != nil {
		return xerrors.Errorf("computing unsealed CID from pieces: %w", err)
	}

	if si.UpdateUnsealed == nil {
		return &ErrBadRU{xerrors.New("nil UpdateUnsealed cid after replica update")}
	}

	if !commD.Equals(*si.UpdateUnsealed) {
		return &ErrBadRU{xerrors.Errorf("calculated CommD differs from updated replica: %s != %s", commD, *si.UpdateUnsealed)}
	}

	if si.UpdateSealed == nil {
		return &ErrBadRU{xerrors.Errorf("nil sealed cid")}
	}
	if si.ReplicaUpdateProof == nil {
		return &ErrBadPR{xerrors.Errorf("nil PR2 proof")}
	}

	return nil
}

func computeUnsealedCIDFromPieces(si SectorInfo) (cid.Cid, error) {
	pcs := si.pieceInfos()
	return proofs.GenerateUnsealedCID(si.SectorType, pcs)
}
