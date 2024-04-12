package seal

import (
	"bytes"
	"context"
	"strings"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type PoRepAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateGetRandomnessFromBeacon(context.Context, crypto.DomainSeparationTag, abi.ChainEpoch, []byte, types.TipSetKey) (abi.Randomness, error)
}

type PoRepTask struct {
	db  *harmonydb.DB
	api PoRepAPI
	sp  *SealPoller
	sc  *ffi.SealCalls

	max int
}

func NewPoRepTask(db *harmonydb.DB, api PoRepAPI, sp *SealPoller, sc *ffi.SealCalls, maxPoRep int) *PoRepTask {
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
		end, err := p.recoverErrors(ctx, sectorParams.SpID, sectorParams.SectorNumber, err)
		if err != nil {
			return false, xerrors.Errorf("recover errors: %w", err)
		}
		if end {
			// done, but the error handling has stored a different than success state
			return true, nil
		}

		return false, xerrors.Errorf("failed to compute seal proof: %w", err)
	}

	// store success!
	n, err := p.db.Exec(ctx, `UPDATE sectors_sdr_pipeline
		SET after_porep = TRUE, seed_value = $3, porep_proof = $4
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

func (p *PoRepTask) recoverErrors(ctx context.Context, spid, snum int64, cerr error) (end bool, err error) {
	const (
		// rust-fil-proofs error strings
		// https://github.com/filecoin-project/rust-fil-proofs/blob/3f018b51b6327b135830899d237a7ba181942d7e/storage-proofs-porep/src/stacked/vanilla/proof.rs#L454C1-L463
		errstrInvalidCommD    = "Invalid comm_d detected at challenge_index"
		errstrInvalidCommR    = "Invalid comm_r detected at challenge_index"
		errstrInvalidEncoding = "Invalid encoding proof generated at layer"
	)

	if cerr == nil {
		return false, xerrors.Errorf("nil error")
	}

	switch {
	case strings.Contains(cerr.Error(), errstrInvalidCommD):
		fallthrough
	case strings.Contains(cerr.Error(), errstrInvalidCommR):
		// todo: it might be more optimal to just retry the Trees compute first.
		//  Invalid CommD/R likely indicates a problem with the data computed in that step
		//  For now for simplicity just retry the whole thing
		fallthrough
	case strings.Contains(cerr.Error(), errstrInvalidEncoding):
		n, err := p.db.Exec(ctx, `UPDATE sectors_sdr_pipeline
		SET after_porep = FALSE, after_sdr = FALSE, after_tree_d = FALSE,
		    after_tree_r = FALSE, after_tree_c = FALSE
		WHERE sp_id = $1 AND sector_number = $2`,
			spid, snum)
		if err != nil {
			return false, xerrors.Errorf("store sdr success: updating pipeline: %w", err)
		}
		if n != 1 {
			return false, xerrors.Errorf("store sdr success: updated %d rows", n)
		}

		return true, nil

	default:
		// if end is false the original error will be returned by the caller
		return false, nil
	}
}

var _ harmonytask.TaskInterface = &PoRepTask{}
