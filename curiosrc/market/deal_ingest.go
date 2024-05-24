package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
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
	AllocatePieceToSector(ctx context.Context, maddr address.Address, piece lpiece.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error)
}

type PieceIngesterApi interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerAllocated(ctx context.Context, a address.Address, key types.TipSetKey) (*bitfield.BitField, error)
	StateNetworkVersion(ctx context.Context, key types.TipSetKey) (network.Version, error)
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
	StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
}

type openSector struct {
	number             abi.SectorNumber
	currentSize        abi.PaddedPieceSize
	earliestStartEpoch abi.ChainEpoch
	index              uint64
	openedAt           *time.Time
	latestEndEpoch     abi.ChainEpoch
}

type PieceIngester struct {
	ctx                 context.Context
	db                  *harmonydb.DB
	api                 PieceIngesterApi
	miner               address.Address
	mid                 uint64 // miner ID
	windowPoStProofType abi.RegisteredPoStProof
	synth               bool
	sectorSize          abi.SectorSize
	sealRightNow        bool // Should be true only for CurioAPI AllocatePieceToSector method
	maxWaitTime         time.Duration
}

type pieceDetails struct {
	Sector     abi.SectorNumber    `db:"sector_number"`
	Size       abi.PaddedPieceSize `db:"piece_size"`
	StartEpoch abi.ChainEpoch      `db:"deal_start_epoch"`
	EndEpoch   abi.ChainEpoch      `db:"deal_end_epoch"`
	Index      uint64              `db:"piece_index"`
	CreatedAt  *time.Time          `db:"created_at"`
}

type verifiedDeal struct {
	isVerified bool
	tmin       abi.ChainEpoch
	tmax       abi.ChainEpoch
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

	pi := &PieceIngester{
		ctx:                 ctx,
		db:                  db,
		api:                 api,
		sealRightNow:        sealRightNow,
		miner:               maddr,
		maxWaitTime:         maxWaitTime,
		sectorSize:          mi.SectorSize,
		windowPoStProofType: mi.WindowPoStProofType,
		mid:                 mid,
		synth:               false, // TODO: synthetic porep config
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
	head, err := p.api.ChainHead(p.ctx)
	if err != nil {
		return xerrors.Errorf("getting chain head: %w", err)
	}

	spt, err := p.getSealProofType()
	if err != nil {
		return xerrors.Errorf("getting seal proof type: %w", err)
	}

	shouldSeal := func(sector *openSector) bool {
		// Start sealing a sector if
		// 1. If sector is full
		// 2. We have been waiting for MaxWaitDuration
		// 3. StartEpoch is less than 8 hours // todo: make this config?
		if sector.currentSize == abi.PaddedPieceSize(p.sectorSize) {
			log.Debugf("start sealing sector %d of miner %d: %s", sector.number, p.miner.String(), "sector full")
			return true
		}
		if time.Since(*sector.openedAt) > p.maxWaitTime {
			log.Debugf("start sealing sector %d of miner %d: %s", sector.number, p.miner.String(), "MaxWaitTime reached")
			return true
		}
		if sector.earliestStartEpoch < head.Height()+abi.ChainEpoch(960) {
			log.Debugf("start sealing sector %d of miner %d: %s", sector.number, p.miner.String(), "earliest start epoch")
			return true
		}
		return false
	}

	comm, err := p.db.BeginTransaction(p.ctx, func(tx *harmonydb.Tx) (commit bool, err error) {

		openSectors, err := p.getOpenSectors(tx)
		if err != nil {
			return false, err
		}

		for _, sector := range openSectors {
			sector := sector
			if shouldSeal(sector) {
				// Start sealing the sector
				cn, err := tx.Exec(`INSERT INTO sectors_sdr_pipeline (sp_id, sector_number, reg_seal_proof) VALUES ($1, $2, $3);`, p.mid, sector.number, spt)

				if err != nil {
					return false, xerrors.Errorf("adding sector to pipeline: %w", err)
				}

				if cn != 1 {
					return false, xerrors.Errorf("adding sector to pipeline: incorrect number of rows returned")
				}

				_, err = tx.Exec("SELECT transfer_and_delete_open_piece($1, $2)", p.mid, sector.number)
				if err != nil {
					return false, xerrors.Errorf("adding sector to pipeline: %w", err)
				}
			}

		}
		return true, nil
	}, harmonydb.OptionRetry())

	if err != nil {
		return xerrors.Errorf("start sealing sector: %w", err)
	}

	if !comm {
		return xerrors.Errorf("start sealing sector: commit failed")
	}

	return nil
}

func (p *PieceIngester) AllocatePieceToSector(ctx context.Context, maddr address.Address, piece lpiece.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error) {
	if maddr != p.miner {
		return api.SectorOffset{}, xerrors.Errorf("miner address doesn't match")
	}

	// check raw size
	if piece.Size() != padreader.PaddedSize(uint64(rawSize)).Padded() {
		return api.SectorOffset{}, xerrors.Errorf("raw size doesn't match padded piece size")
	}

	var propJson []byte

	dataHdrJson, err := json.Marshal(header)
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("json.Marshal(header): %w", err)
	}

	vd := verifiedDeal{
		isVerified: false,
	}

	if piece.DealProposal != nil {
		vd.isVerified = piece.DealProposal.VerifiedDeal
		if vd.isVerified {
			alloc, err := p.api.StateGetAllocationForPendingDeal(ctx, piece.DealID, types.EmptyTSK)
			if err != nil {
				return api.SectorOffset{}, xerrors.Errorf("getting pending allocation for deal %d: %w", piece.DealID, err)
			}
			if alloc == nil {
				return api.SectorOffset{}, xerrors.Errorf("no allocation found for deal %d: %w", piece.DealID, err)
			}
			vd.tmin = alloc.TermMin
			vd.tmax = alloc.TermMax
		}
		propJson, err = json.Marshal(piece.DealProposal)
		if err != nil {
			return api.SectorOffset{}, xerrors.Errorf("json.Marshal(piece.DealProposal): %w", err)
		}
	} else {
		vd.isVerified = piece.PieceActivationManifest.VerifiedAllocationKey != nil
		if vd.isVerified {
			client, err := address.NewIDAddress(uint64(piece.PieceActivationManifest.VerifiedAllocationKey.Client))
			if err != nil {
				return api.SectorOffset{}, xerrors.Errorf("getting client address from actor ID: %w", err)
			}
			alloc, err := p.api.StateGetAllocation(ctx, client, verifregtypes.AllocationId(piece.PieceActivationManifest.VerifiedAllocationKey.ID), types.EmptyTSK)
			if err != nil {
				return api.SectorOffset{}, xerrors.Errorf("getting allocation details for %d: %w", piece.PieceActivationManifest.VerifiedAllocationKey.ID, err)
			}
			if alloc == nil {
				return api.SectorOffset{}, xerrors.Errorf("no allocation found for ID %d: %w", piece.PieceActivationManifest.VerifiedAllocationKey.ID, err)
			}
			vd.tmin = alloc.TermMin
			vd.tmax = alloc.TermMax
		}
		propJson, err = json.Marshal(piece.PieceActivationManifest)
		if err != nil {
			return api.SectorOffset{}, xerrors.Errorf("json.Marshal(piece.PieceActivationManifest): %w", err)
		}
	}

	if !p.sealRightNow {
		// Try to allocate the piece to an open sector
		allocated, ret, err := p.allocateToExisting(ctx, piece, rawSize, source, dataHdrJson, propJson, vd)
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
			_, err = tx.Exec(`SELECT insert_sector_market_piece($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
				p.mid, n, 0,
				piece.DealProposal.PieceCID, piece.DealProposal.PieceSize,
				source.String(), dataHdrJson, rawSize, !piece.KeepUnsealed,
				piece.PublishCid, piece.DealID, propJson, piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch)
			if err != nil {
				return false, xerrors.Errorf("adding deal to sector: %w", err)
			}
		} else {
			_, err = tx.Exec(`SELECT insert_sector_ddo_piece($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
				p.mid, n, 0,
				piece.PieceActivationManifest.CID, piece.PieceActivationManifest.Size,
				source.String(), dataHdrJson, rawSize, !piece.KeepUnsealed,
				piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch, propJson)
			if err != nil {
				return false, xerrors.Errorf("adding deal to sector: %w", err)
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

	if p.sealRightNow {
		err = p.SectorStartSealing(ctx, num[0])
		if err != nil {
			return api.SectorOffset{}, xerrors.Errorf("SectorStartSealing: %w", err)
		}
	}

	return api.SectorOffset{
		Sector: num[0],
		Offset: 0,
	}, nil
}

func (p *PieceIngester) allocateToExisting(ctx context.Context, piece lpiece.PieceDealInfo, rawSize int64, source url.URL, dataHdrJson, propJson []byte, vd verifiedDeal) (bool, api.SectorOffset, error) {

	var ret api.SectorOffset
	var allocated bool
	var rerr error

	comm, err := p.db.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
		openSectors, err := p.getOpenSectors(tx)
		if err != nil {
			return false, err
		}

		pieceSize := piece.Size()
		for _, sec := range openSectors {
			sec := sec
			if sec.currentSize+pieceSize <= abi.PaddedPieceSize(p.sectorSize) {
				if vd.isVerified {
					sectorLifeTime := sec.latestEndEpoch - sec.earliestStartEpoch
					// Allocation's TMin must fit in sector and TMax should be at least sector lifetime or more
					// Based on https://github.com/filecoin-project/builtin-actors/blob/a0e34d22665ac8c84f02fea8a099216f29ffaeeb/actors/verifreg/src/lib.rs#L1071-L1086
					if sectorLifeTime <= vd.tmin && sectorLifeTime >= vd.tmax {
						continue
					}
				}

				ret.Sector = sec.number
				ret.Offset = sec.currentSize

				// Insert market deal to DB for the sector
				if piece.DealProposal != nil {
					cn, err := tx.Exec(`SELECT insert_sector_market_piece($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
						p.mid, sec.number, sec.index+1,
						piece.DealProposal.PieceCID, piece.DealProposal.PieceSize,
						source.String(), dataHdrJson, rawSize, !piece.KeepUnsealed,
						piece.PublishCid, piece.DealID, propJson, piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch)

					if err != nil {
						return false, fmt.Errorf("adding deal to sector: %v", err)
					}

					if cn != 1 {
						return false, xerrors.Errorf("expected one piece")
					}

				} else { // Insert DDO deal to DB for the sector
					cn, err := tx.Exec(`SELECT insert_sector_ddo_piece($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
						p.mid, sec.number, sec.index+1,
						piece.PieceActivationManifest.CID, piece.PieceActivationManifest.Size,
						source.String(), dataHdrJson, rawSize, !piece.KeepUnsealed,
						piece.DealSchedule.StartEpoch, piece.DealSchedule.EndEpoch, propJson)

					if err != nil {
						return false, fmt.Errorf("adding deal to sector: %v", err)
					}

					if cn != 1 {
						return false, xerrors.Errorf("expected one piece")
					}

				}
				allocated = true
				break
			}
		}
		return true, nil
	}, harmonydb.OptionRetry())

	if !comm {
		rerr = xerrors.Errorf("allocating piece to a sector: commit failed")
	}

	if err != nil {
		rerr = xerrors.Errorf("allocating piece to a sector: %w", err)
	}

	return allocated, ret, rerr

}

func (p *PieceIngester) SectorStartSealing(ctx context.Context, sector abi.SectorNumber) error {

	spt, err := p.getSealProofType()
	if err != nil {
		return xerrors.Errorf("getting seal proof type: %w", err)
	}

	comm, err := p.db.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
		// Get current open sector pieces from DB
		var pieces []pieceDetails
		err = tx.Select(&pieces, `
					SELECT
					    sector_number,
						piece_size,
						piece_index,
						COALESCE(direct_start_epoch, f05_deal_start_epoch, 0) AS deal_start_epoch,
						COALESCE(direct_end_epoch, f05_deal_end_epoch, 0) AS deal_end_epoch,
						created_at
					FROM
						open_sector_pieces
					WHERE
						sp_id = $1 AND sector_number = $2
					ORDER BY
						piece_index DESC;`, p.mid, sector)
		if err != nil {
			return false, xerrors.Errorf("getting open sectors from DB")
		}

		if len(pieces) < 1 {
			return false, xerrors.Errorf("sector %d is not waiting to be sealed", sector)
		}

		cn, err := tx.Exec(`INSERT INTO sectors_sdr_pipeline (sp_id, sector_number, reg_seal_proof) VALUES ($1, $2, $3);`, p.mid, sector, spt)

		if err != nil {
			return false, xerrors.Errorf("adding sector to pipeline: %w", err)
		}

		if cn != 1 {
			return false, xerrors.Errorf("incorrect number of rows returned")
		}

		_, err = tx.Exec("SELECT transfer_and_delete_open_piece($1, $2)", p.mid, sector)
		if err != nil {
			return false, xerrors.Errorf("adding sector to pipeline: %w", err)
		}

		return true, nil

	}, harmonydb.OptionRetry())

	if err != nil {
		return xerrors.Errorf("start sealing sector: %w", err)
	}

	if !comm {
		return xerrors.Errorf("start sealing sector: commit failed")
	}

	return nil
}

func (p *PieceIngester) getOpenSectors(tx *harmonydb.Tx) ([]*openSector, error) {
	// Get current open sector pieces from DB
	var pieces []pieceDetails
	err := tx.Select(&pieces, `
					SELECT
					    sector_number,
						piece_size,
						piece_index,
						COALESCE(direct_start_epoch, f05_deal_start_epoch, 0) AS deal_start_epoch,
						COALESCE(direct_end_epoch, f05_deal_end_epoch, 0) AS deal_end_epoch,
						created_at
					FROM
						open_sector_pieces
					WHERE
						sp_id = $1
					ORDER BY
						piece_index DESC;`, p.mid)
	if err != nil {
		return nil, xerrors.Errorf("getting open sectors from DB")
	}

	getStartEpoch := func(new abi.ChainEpoch, cur abi.ChainEpoch) abi.ChainEpoch {
		if cur > 0 && cur < new {
			return cur
		}
		return new
	}

	getEndEpoch := func(new abi.ChainEpoch, cur abi.ChainEpoch) abi.ChainEpoch {
		if cur > 0 && cur > new {
			return cur
		}
		return new
	}

	getOpenedAt := func(piece pieceDetails, cur *time.Time) *time.Time {
		if piece.CreatedAt.Before(*cur) {
			return piece.CreatedAt
		}
		return cur
	}

	sectorMap := map[abi.SectorNumber]*openSector{}
	for _, pi := range pieces {
		pi := pi
		sector, ok := sectorMap[pi.Sector]
		if !ok {
			sectorMap[pi.Sector] = &openSector{
				number:             pi.Sector,
				currentSize:        pi.Size,
				earliestStartEpoch: getStartEpoch(pi.StartEpoch, 0),
				index:              pi.Index,
				openedAt:           pi.CreatedAt,
				latestEndEpoch:     getEndEpoch(pi.EndEpoch, 0),
			}
			continue
		}
		sector.currentSize += pi.Size
		sector.earliestStartEpoch = getStartEpoch(pi.StartEpoch, sector.earliestStartEpoch)
		sector.latestEndEpoch = getEndEpoch(pi.EndEpoch, sector.earliestStartEpoch)
		if sector.index < pi.Index {
			sector.index = pi.Index
		}
		sector.openedAt = getOpenedAt(pi, sector.openedAt)
	}

	var os []*openSector

	for _, v := range sectorMap {
		v := v
		os = append(os, v)
	}

	return os, nil
}

func (p *PieceIngester) getSealProofType() (abi.RegisteredSealProof, error) {
	nv, err := p.api.StateNetworkVersion(p.ctx, types.EmptyTSK)
	if err != nil {
		return 0, xerrors.Errorf("getting network version: %w", err)
	}

	return miner.PreferredSealProofTypeFromWindowPoStType(nv, p.windowPoStProofType, p.synth)
}
