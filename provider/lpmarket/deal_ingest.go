package lpmarket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/provider/lpseal"
)

type Ingester interface {
	AllocatePieceToSector(ctx context.Context, maddr address.Address, piece api.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error)
}

type PieceIngesterApi interface {
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerAllocated(ctx context.Context, a address.Address, key types.TipSetKey) (*bitfield.BitField, error)
	StateNetworkVersion(ctx context.Context, key types.TipSetKey) (network.Version, error)
}

type PieceIngester struct {
	db  *harmonydb.DB
	api PieceIngesterApi
}

func NewPieceIngester(db *harmonydb.DB, api PieceIngesterApi) *PieceIngester {
	return &PieceIngester{db: db, api: api}
}

func (p *PieceIngester) AllocatePieceToSector(ctx context.Context, maddr address.Address, piece api.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error) {
	mi, err := p.api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return api.SectorOffset{}, err
	}

	if piece.DealProposal.PieceSize != abi.PaddedPieceSize(mi.SectorSize) {
		return api.SectorOffset{}, xerrors.Errorf("only full sector pieces supported for now")
	}

	// check raw size
	if piece.DealProposal.PieceSize != padreader.PaddedSize(uint64(rawSize)).Padded() {
		return api.SectorOffset{}, xerrors.Errorf("raw size doesn't match padded piece size")
	}

	// add initial piece + to a sector
	nv, err := p.api.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("getting network version: %w", err)
	}

	synth := false // todo synthetic porep config
	spt, err := miner.PreferredSealProofTypeFromWindowPoStType(nv, mi.WindowPoStProofType, synth)
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("getting seal proof type: %w", err)
	}

	num, err := lpseal.AllocateSectorNumbers(ctx, p.api, p.db, maddr, 1, func(tx *harmonydb.Tx, numbers []abi.SectorNumber) (bool, error) {
		if len(numbers) != 1 {
			return false, xerrors.Errorf("expected one sector number")
		}
		n := numbers[0]

		_, err := tx.Exec("insert into sectors_sdr_pipeline (sp_id, sector_number, reg_seal_proof) values ($1, $2, $3)", maddr, n, spt)
		if err != nil {
			return false, xerrors.Errorf("inserting into sectors_sdr_pipeline: %w", err)
		}

		dataHdrJson, err := json.Marshal(header)
		if err != nil {
			return false, xerrors.Errorf("json.Marshal(header): %w", err)
		}

		dealProposalJson, err := json.Marshal(piece.DealProposal)
		if err != nil {
			return false, xerrors.Errorf("json.Marshal(piece.DealProposal): %w", err)
		}

		_, err = tx.Exec(`INSERT INTO sectors_sdr_initial_pieces (sp_id,
                                        sector_number,
                                        piece_index,
                                        
                                        piece_cid,
                                        piece_size,
                                        
                                        data_url,
                                        data_headers,
                                        data_raw_size,
                                        data_delete_on_finalize,
                                        
                                        f05_publish_cid,
                                        f05_deal_id,
                                        f05_deal_proposal,
                                        f05_deal_start_epoch,
                                        f05_deal_end_epoch) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			maddr, n, 0,
			piece.DealProposal.PieceCID, piece.DealProposal.PieceSize,
			source.String(), dataHdrJson, rawSize, true,
			piece.PublishCid, piece.DealID, dealProposalJson, piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch)
		if err != nil {
			return false, xerrors.Errorf("inserting into sectors_sdr_initial_pieces: %w", err)
		}

		return true, nil
	})
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("allocating sector numbers: %w", err)
	}

	if len(num) != 1 {
		return api.SectorOffset{}, xerrors.Errorf("expected one sector number")
	}

	// After we insert the piece/sector_pipeline entries, the lpseal/poller will take it from here

	return api.SectorOffset{
		Sector: num[0],
		Offset: 0,
	}, nil
}
