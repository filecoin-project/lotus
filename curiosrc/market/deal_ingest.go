package market

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/curiosrc/seal"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	lpiece "github.com/filecoin-project/lotus/storage/pipeline/piece"
)

var log = logging.Logger("piece-ingestor")

const loopFrequency = 10 * time.Second

type Ingester interface {
	AllocatePieceToSector(ctx context.Context, maddr address.Address, piece api.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error)
}

type PieceIngesterApi interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerAllocated(ctx context.Context, a address.Address, key types.TipSetKey) (*bitfield.BitField, error)
	StateNetworkVersion(ctx context.Context, key types.TipSetKey) (network.Version, error)
}

type openSector struct {
	number        abi.SectorNumber
	currentSize   abi.PaddedPieceSize
	earliestEpoch abi.ChainEpoch
	index         uint64
	lk            sync.Mutex
	openedAt      time.Time
	sealNow       bool
}

type PieceIngester struct {
	ctx                 context.Context
	db                  *harmonydb.DB
	api                 PieceIngesterApi
	miner               address.Address
	mid                 uint64 // miner ID
	windowPoStProofType abi.RegisteredPoStProof
	sectorSize          abi.SectorSize
	sealRightNow        bool // Should be true only for CurioAPI AllocatePieceToSector method
	maxWaitTime         time.Duration
	forceSeal           chan struct{}
	openSectors         []*openSector
	lk                  sync.Mutex
}

func NewPieceIngester(ctx context.Context, db *harmonydb.DB, api PieceIngesterApi, maddr address.Address, sealRightNow bool, maxWaitTime time.Duration) (*PieceIngester, error) {
	mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return nil, xerrors.Errorf("getting miner ID: %w", err)
	}

	sm := map[abi.SectorNumber]*openSector{}
	type details struct {
		Sector   abi.SectorNumber    `db:"sector_number"`
		Size     abi.PaddedPieceSize `db:"piece_size"`
		f05Epoch abi.ChainEpoch      `db:"f05_deal_start_epoch"`
		ddoEpoch abi.ChainEpoch      `db:"direct_start_epoch"`
		Index    uint64              `db:"piece_index"`
	}
	var pieces []details

	// Get current open sector pieces from DB
	err = db.Select(ctx, &pieces, `
					SELECT
					    ssip.sector_number,
						ssip.piece_size,
						ssip.piece_index,
						ssip.f05_deal_start_epoch,
						ssip.direct_start_epoch
					FROM
						curio.sectors_sdr_initial_pieces ssip
					JOIN
						(SELECT sector_number
						 FROM curio.sectors_sdr_initial_pieces sip
						 LEFT JOIN curio.sectors_sdr_pipeline sp ON sip.sp_id = sp.sp_id AND sip.sector_number = sp.sector_number
						 WHERE sp.sector_number IS NULL AND sip.sp_id = $1) as filtered ON ssip.sector_number = filtered.sector_number
					WHERE
						ssip.sp_id = $1
					ORDER BY
						ssip.piece_index DESC;`, mid)
	if err != nil {
		return nil, xerrors.Errorf("getting open sectors from DB")
	}

	for _, piece := range pieces {
		var ep abi.ChainEpoch
		if piece.ddoEpoch > 0 {
			ep = piece.ddoEpoch
		} else {
			ep = piece.f05Epoch
		}
		s, ok := sm[piece.Sector]
		if !ok {
			s := &openSector{
				number:        piece.Sector,
				currentSize:   piece.Size,
				earliestEpoch: ep,
				index:         piece.Index,
				openedAt:      time.Now(),
			}
			sm[piece.Sector] = s
		}
		s.currentSize += piece.Size
		s.earliestEpoch = ep
		if s.index < piece.Index {
			s.index = piece.Index
		}
	}

	var os []*openSector
	for _, v := range sm {
		os = append(os, v)
	}

	pi := &PieceIngester{
		db:                  db,
		api:                 api,
		sealRightNow:        sealRightNow,
		miner:               maddr,
		maxWaitTime:         maxWaitTime,
		sectorSize:          mi.SectorSize,
		windowPoStProofType: mi.WindowPoStProofType,
		openSectors:         os,
		mid:                 mid,
		forceSeal:           make(chan struct{}, 1),
	}

	go pi.start()

	return pi, nil
}

func (p *PieceIngester) start() {
	ticker := time.NewTicker(loopFrequency)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			err := p.Seal()
			if err != nil {
				log.Error(err)
			}
		}
	}
}

func (p *PieceIngester) Seal() error {
	p.lk.Lock()
	defer p.lk.Unlock()

	head, err := p.api.ChainHead(p.ctx)
	if err != nil {
		return xerrors.Errorf("getting chain head: %w", err)
	}

	nv, err := p.api.StateNetworkVersion(p.ctx, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting network version: %w", err)
	}

	synth := false // todo synthetic porep config

	spt, err := miner.PreferredSealProofTypeFromWindowPoStType(nv, p.windowPoStProofType, synth)
	if err != nil {
		return xerrors.Errorf("getting seal proof type: %w", err)
	}

	var os []*openSector

	for i := range p.openSectors {
		// todo: When to start sealing
		// Start sealing a sector if
		// 1. If sector is full
		// 2. We have been waiting for MaxWaitDuration
		// 3. StartEpoch is less than 8 hours // todo: make this config?
		// 4. If force seal is set //todo: create sealNOW command
		if p.openSectors[i].sealNow || p.openSectors[i].currentSize == abi.PaddedPieceSize(p.sectorSize) || time.Since(p.openSectors[i].openedAt) >= p.maxWaitTime || p.openSectors[i].earliestEpoch > head.Height()+abi.ChainEpoch(960) {
			// Start sealing the sector
			p.openSectors[i].lk.Lock()
			defer p.openSectors[i].lk.Unlock()

			cn, err := p.db.Exec(p.ctx, `
					INSERT INTO sectors_sdr_pipeline (sp_id, sector_number, reg_seal_proof) VALUES ($1, $2, $3);`, p.mid, p.openSectors[i].number, spt)

			if err != nil {
				return xerrors.Errorf("adding sector to pipeline: %w", err)
			}

			if cn != 1 {
				return xerrors.Errorf("incorrect number of rows returned")
			}
		}
		os = append(os, p.openSectors[i])
	}

	p.openSectors = os

	return nil
}

func (p *PieceIngester) AllocatePieceToSector(ctx context.Context, maddr address.Address, piece lpiece.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error) {
	if maddr != p.miner {
		return api.SectorOffset{}, xerrors.Errorf("miner address doesn't match")
	}

	if piece.DealProposal.PieceSize != abi.PaddedPieceSize(p.sectorSize) {
		return api.SectorOffset{}, xerrors.Errorf("only full sector pieces supported for now")
	}

	// check raw size
	if piece.DealProposal.PieceSize != padreader.PaddedSize(uint64(rawSize)).Padded() {
		return api.SectorOffset{}, xerrors.Errorf("raw size doesn't match padded piece size")
	}

	var propJson []byte

	dataHdrJson, err := json.Marshal(header)
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("json.Marshal(header): %w", err)
	}

	if piece.DealProposal != nil {
		propJson, err = json.Marshal(piece.DealProposal)
		if err != nil {
			return api.SectorOffset{}, xerrors.Errorf("json.Marshal(piece.DealProposal): %w", err)
		}
	} else {
		propJson, err = json.Marshal(piece.PieceActivationManifest)
		if err != nil {
			return api.SectorOffset{}, xerrors.Errorf("json.Marshal(piece.PieceActivationManifest): %w", err)
		}
	}

	p.lk.Lock()
	defer p.lk.Unlock()

	if !p.sealRightNow {
		// Try to allocate the piece to an open sector
		allocated, ret, err := p.allocateToExisting(ctx, piece, rawSize, source, dataHdrJson, propJson)
		if err != nil {
			return api.SectorOffset{}, err
		}
		if allocated {
			return ret, nil
		}
	}

	// Allocation to open sector failed, create a new sector and add the piece to it
	num, err := seal.AllocateSectorNumbers(ctx, p.api, p.db, maddr, 1, func(tx *harmonydb.Tx, numbers []abi.SectorNumber) (bool, error) {
		if len(numbers) != 1 {
			return false, xerrors.Errorf("expected one sector number")
		}
		n := numbers[0]

		if piece.DealProposal != nil {
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
				p.mid, n, 0,
				piece.DealProposal.PieceCID, piece.DealProposal.PieceSize,
				source.String(), dataHdrJson, rawSize, !piece.KeepUnsealed,
				piece.PublishCid, piece.DealID, propJson, piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch)
			if err != nil {
				return false, xerrors.Errorf("inserting into sectors_sdr_initial_pieces: %w", err)
			}
		} else {
			_, err = tx.Exec(`INSERT INTO sectors_sdr_initial_pieces (sp_id,
                                        sector_number,
                                        piece_index,
                                        
                                        piece_cid,
                                        piece_size,
                                        
                                        data_url,
                                        data_headers,
                                        data_raw_size,
                                        data_delete_on_finalize,
                                        
                                        direct_start_epoch,
                                        direct_end_epoch,
                                        direct_piece_activation_manifest) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
				p.mid, n, 0,
				piece.DealProposal.PieceCID, piece.DealProposal.PieceSize,
				source.String(), dataHdrJson, rawSize, !piece.KeepUnsealed,
				piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch, propJson)
			if err != nil {
				return false, xerrors.Errorf("inserting into sectors_sdr_initial_pieces: %w", err)
			}
		}
		return true, nil
	})
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("allocating sector numbers: %w", err)
	}

	if len(num) != 1 {
		return api.SectorOffset{}, xerrors.Errorf("expected one sector number")
	}

	sec := &openSector{
		number:        num[0],
		currentSize:   piece.DealProposal.PieceSize,
		earliestEpoch: piece.DealProposal.StartEpoch,
		index:         0,
		openedAt:      time.Now(),
	}

	if p.sealRightNow {
		sec.sealNow = true
	}

	p.openSectors = append(p.openSectors, sec)

	return api.SectorOffset{
		Sector: num[0],
		Offset: 0,
	}, nil
}

func (p *PieceIngester) allocateToExisting(ctx context.Context, piece lpiece.PieceDealInfo, rawSize int64, source url.URL, dataHdrJson, propJson []byte) (bool, api.SectorOffset, error) {
	for i := range p.openSectors {
		sec := p.openSectors[i]
		sec.lk.Lock()
		defer sec.lk.Unlock()
		pieceSize := piece.Size()
		if sec.currentSize+pieceSize < abi.PaddedPieceSize(p.sectorSize) {

			ret := api.SectorOffset{
				Sector: sec.number,
				Offset: sec.currentSize + 1, // Current filled up size + 1
			}

			// Insert market deal to DB for the sector
			if piece.DealProposal != nil {
				cn, err := p.db.Exec(ctx, `INSERT INTO sectors_sdr_initial_pieces (sp_id,
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
					p.mid, sec.number, sec.index+1,
					piece.DealProposal.PieceCID, piece.DealProposal.PieceSize,
					source.String(), dataHdrJson, rawSize, !piece.KeepUnsealed,
					piece.PublishCid, piece.DealID, propJson, piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch)

				if err != nil {
					return false, api.SectorOffset{}, xerrors.Errorf("adding deal to sector: %w", err)
				}

				if cn != 1 {
					return false, api.SectorOffset{}, xerrors.Errorf("expected one piece")
				}
			} else { // Insert DDO deal to DB for the sector
				cn, err := p.db.Exec(ctx, `INSERT INTO sectors_sdr_initial_pieces (sp_id,
                                        sector_number,
                                        piece_index,
                                        
                                        piece_cid,
                                        piece_size,
                                        
                                        data_url,
                                        data_headers,
                                        data_raw_size,
                                        data_delete_on_finalize,
                                        
                                        direct_start_epoch,
                                        direct_end_epoch,
                                        direct_piece_activation_manifest) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
					p.mid, sec.number, sec.index+1,
					piece.DealProposal.PieceCID, piece.DealProposal.PieceSize,
					source.String(), dataHdrJson, rawSize, !piece.KeepUnsealed,
					piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch, propJson)

				if err != nil {
					return false, api.SectorOffset{}, xerrors.Errorf("adding deal to sector: %w", err)
				}

				if cn != 1 {
					return false, api.SectorOffset{}, xerrors.Errorf("expected one piece")
				}
			}
			sec.currentSize += pieceSize
			startEpoch, _ := piece.StartEpoch() // ignoring error as method always returns nil
			if sec.earliestEpoch > startEpoch {
				sec.earliestEpoch = startEpoch
			}
			sec.index++
			return true, ret, nil
		}
	}
	return false, api.SectorOffset{}, nil
}

func (p *PieceIngester) SectorStartSealing(ctx context.Context, sector abi.SectorNumber) error {
	p.lk.Lock()
	defer p.lk.Unlock()
	for _, s := range p.openSectors {
		s := s
		if s.number == sector {
			s.lk.Lock()
			s.sealNow = true
			s.lk.Unlock()
			return nil
		}
	}
	return xerrors.Errorf("sector not found")
}
