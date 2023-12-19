package lpseal

import (
	"bytes"
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/provider/lpffi"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"golang.org/x/xerrors"
)

type SDRAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateGetRandomnessFromTickets(context.Context, crypto.DomainSeparationTag, abi.ChainEpoch, []byte, types.TipSetKey) (abi.Randomness, error)
}

type SDRTask struct {
	api SDRAPI
	db  *harmonydb.DB
	sp  *SealPoller

	sc *lpffi.SealCalls

	maxSDR int
}

func NewSDRTask(api SDRAPI, db *harmonydb.DB, sp *SealPoller, sc *lpffi.SealCalls, maxSDR int) *SDRTask {
	return &SDRTask{
		api:    api,
		db:     db,
		sp:     sp,
		sc:     sc,
		maxSDR: maxSDR,
	}
}

func (s *SDRTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var sectorParams struct {
		SpID         int64                   `db:"sp_id"`
		SectorNumber int64                   `db:"sector_number"`
		RegSealProof abi.RegisteredSealProof `db:"reg_seal_proof"`
	}

	err = s.db.Select(ctx, &sectorParams, `
		SELECT sp_id, sector_number, reg_seal_proof
		FROM sectors_sdr_pipeline
		WHERE task_id_sdr = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting sector params: %w", err)
	}

	var pieces []struct {
		PieceIndex int64  `db:"piece_index"`
		PieceCID   string `db:"piece_cid"`
		PieceSize  int64  `db:"piece_size"`
	}

	err = s.db.Select(ctx, &pieces, `
		SELECT piece_index, piece_cid, piece_size
		FROM sectors_sdr_initial_pieces
		WHERE sp_id = $1 AND sector_number = $2`, sectorParams.SpID, sectorParams.SectorNumber)
	if err != nil {
		return false, xerrors.Errorf("getting pieces: %w", err)
	}

	if len(pieces) > 0 {
		// todo sdr with data
		return false, xerrors.Errorf("todo sdr with data")
	}

	ssize, err := sectorParams.RegSealProof.SectorSize()
	if err != nil {
		return false, xerrors.Errorf("getting sector size: %w", err)
	}

	commd := zerocomm.ZeroPieceCommitment(abi.PaddedPieceSize(ssize).Unpadded())

	sref := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(sectorParams.SpID),
			Number: abi.SectorNumber(sectorParams.SectorNumber),
		},
		ProofType: sectorParams.RegSealProof,
	}

	// get ticket
	maddr, err := address.NewIDAddress(uint64(sectorParams.SpID))
	if err != nil {
		return false, xerrors.Errorf("getting miner address: %w", err)
	}

	ticket, err := s.getTicket(ctx, maddr)
	if err != nil {
		return false, xerrors.Errorf("getting ticket: %w", err)
	}

	// do the SDR!!

	err = s.sc.GenerateSDR(ctx, sref, ticket, commd)
	if err != nil {
		return false, xerrors.Errorf("generating sdr: %w", err)
	}

	// store success!
	n, err := s.db.Exec(ctx, `UPDATE sectors_sdr_pipeline SET after_sdr = true WHERE sp_id = $1 AND sector_number = $2`, sectorParams.SpID, sectorParams.SectorNumber)
	if err != nil {
		return false, xerrors.Errorf("store sdr success: updating pipeline: %w", err)
	}
	if n != 1 {
		return false, xerrors.Errorf("store sdr success: updated %d rows", n)
	}

	return true, nil
}

func (s *SDRTask) getTicket(ctx context.Context, maddr address.Address) (abi.SealRandomness, error) {
	ts, err := s.api.ChainHead(ctx)
	if err != nil {
		return nil, xerrors.Errorf("getting chain head: %w", err)
	}

	ticketEpoch := ts.Height() - policy.SealRandomnessLookback
	buf := new(bytes.Buffer)
	if err := maddr.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("marshaling miner address: %w", err)
	}

	rand, err := s.api.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_SealRandomness, ticketEpoch, buf.Bytes(), ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting randomness from tickets: %w", err)
	}

	return abi.SealRandomness(rand), nil
}

func (s *SDRTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	// todo check storage (reserve too?)

	id := ids[0]
	return &id, nil
}

func (s *SDRTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  s.maxSDR,
		Name: "SDR",
		Cost: resources.Resources{ // todo offset for prefetch?
			Cpu: 4, // todo multicore sdr
			Gpu: 0,
			Ram: 54 << 30, // todo measure; lower on 2k devnetn
		},
		MaxFailures: 0,
		Follows:     nil,
	}
}

func (s *SDRTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	s.sp.pollers[pollerSDR].Set(taskFunc)
}

var _ harmonytask.TaskInterface = &SDRTask{}
