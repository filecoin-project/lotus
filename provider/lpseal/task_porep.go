package lpseal

import (
	"bytes"
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/provider/lpffi"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

type PoRepAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateGetRandomnessFromBeacon(context.Context, crypto.DomainSeparationTag, abi.ChainEpoch, []byte, types.TipSetKey) (abi.Randomness, error)
}

type PoRepTask struct {
	db  *harmonydb.DB
	api PoRepAPI
	sp  *SealPoller
	sc  *lpffi.SealCalls

	max int
}

func NewPoRepTask(db *harmonydb.DB, api PoRepAPI, sp *SealPoller, sc *lpffi.SealCalls, maxPoRep int) *PoRepTask {
	return &PoRepTask{
		db:  db,
		api: api,
		sp:  sp,
		sc:  sc,
		max: maxPoRep,
	}
}

func (p *PoRepTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var sectorParamsArr []struct {
		SpID         int64                   `db:"sp_id"`
		SectorNumber int64                   `db:"sector_number"`
		RegSealProof abi.RegisteredSealProof `db:"reg_seal_proof"`
		TicketEpoch  abi.ChainEpoch          `db:"ticket_epoch"`
		TicketValue  []byte                  `db:"ticket_value"`
		SeedEpoch    abi.ChainEpoch          `db:"seed_epoch"`
		SealedCID    string                  `db:"tree_r_cid"`
		UnsealedCID  string                  `db:"tree_d_cid"`
	}

	err = p.db.Select(ctx, &sectorParamsArr, `
		SELECT sp_id, sector_number, reg_seal_proof, ticket_epoch, ticket_value, seed_epoch, tree_r_cid, tree_d_cid
		FROM sectors_sdr_pipeline
		WHERE task_id_porep = $1`, taskID)
	if err != nil {
		return false, err
	}
	if len(sectorParamsArr) != 1 {
		return false, xerrors.Errorf("expected 1 sector params, got %d", len(sectorParamsArr))
	}
	sectorParams := sectorParamsArr[0]

	sealed, err := cid.Parse(sectorParams.SealedCID)
	if err != nil {
		return false, xerrors.Errorf("failed to parse sealed cid: %w", err)
	}

	unsealed, err := cid.Parse(sectorParams.UnsealedCID)
	if err != nil {
		return false, xerrors.Errorf("failed to parse unsealed cid: %w", err)
	}

	ts, err := p.api.ChainHead(ctx)
	if err != nil {
		return false, xerrors.Errorf("failed to get chain head: %w", err)
	}

	maddr, err := address.NewIDAddress(uint64(sectorParams.SpID))
	if err != nil {
		return false, xerrors.Errorf("failed to create miner address: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := maddr.MarshalCBOR(buf); err != nil {
		return false, xerrors.Errorf("failed to marshal miner address: %w", err)
	}

	rand, err := p.api.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, sectorParams.SeedEpoch, buf.Bytes(), ts.Key())
	if err != nil {
		return false, xerrors.Errorf("failed to get randomness for computing seal proof: %w", err)
	}

	sr := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(sectorParams.SpID),
			Number: abi.SectorNumber(sectorParams.SectorNumber),
		},
		ProofType: sectorParams.RegSealProof,
	}

	// COMPUTE THE PROOF!

	proof, err := p.sc.PoRepSnark(ctx, sr, sealed, unsealed, sectorParams.TicketValue, abi.InteractiveSealRandomness(rand))
	if err != nil {
		return false, xerrors.Errorf("failed to compute seal proof: %w", err)
	}

	// store success!
	n, err := p.db.Exec(ctx, `UPDATE sectors_sdr_pipeline
		SET after_sdr = true, seed_value = $3, porep_proof = $4
		WHERE sp_id = $1 AND sector_number = $2`,
		sectorParams.SpID, sectorParams.SectorNumber, []byte(rand), proof)
	if err != nil {
		return false, xerrors.Errorf("store sdr success: updating pipeline: %w", err)
	}
	if n != 1 {
		return false, xerrors.Errorf("store sdr success: updated %d rows", n)
	}

	return true, nil
}

func (p *PoRepTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	// todo sort by priority

	id := ids[0]
	return &id, nil
}

func (p *PoRepTask) TypeDetails() harmonytask.TaskTypeDetails {
	res := harmonytask.TaskTypeDetails{
		Max:  p.max,
		Name: "PoRep",
		Cost: resources.Resources{
			Cpu:       1,
			Gpu:       1,
			Ram:       50 << 30, // todo correct value
			MachineID: 0,
		},
		MaxFailures: 5,
		Follows:     nil,
	}

	if isDevnet {
		res.Cost.Ram = 1 << 30
	}

	return res
}

func (p *PoRepTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	p.sp.pollers[pollerPoRep].Set(taskFunc)
}

var _ harmonytask.TaskInterface = &PoRepTask{}
