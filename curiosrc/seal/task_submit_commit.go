package seal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner2 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	verifreg13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	verifregtypes9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/curiosrc/message"
	"github.com/filecoin-project/lotus/curiosrc/multictladdr"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/ctladdr"
)

type SubmitCommitAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (big.Int, error)
	StateSectorPreCommitInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error)
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes9.AllocationId, tsk types.TipSetKey) (*verifregtypes9.Allocation, error)
	StateGetAllocationIdForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (verifregtypes9.AllocationId, error)
	ctladdr.NodeApi
}

type commitConfig struct {
	maxFee                     types.FIL
	RequireActivationSuccess   bool
	RequireNotificationSuccess bool
}

type SubmitCommitTask struct {
	sp  *SealPoller
	db  *harmonydb.DB
	api SubmitCommitAPI

	sender *message.Sender
	as     *multictladdr.MultiAddressSelector
	cfg    commitConfig
}

func NewSubmitCommitTask(sp *SealPoller, db *harmonydb.DB, api SubmitCommitAPI, sender *message.Sender, as *multictladdr.MultiAddressSelector, cfg *config.CurioConfig) *SubmitCommitTask {

	cnfg := commitConfig{
		maxFee:                     cfg.Fees.MaxCommitGasFee,
		RequireActivationSuccess:   cfg.Subsystems.RequireActivationSuccess,
		RequireNotificationSuccess: cfg.Subsystems.RequireNotificationSuccess,
	}

	return &SubmitCommitTask{
		sp:     sp,
		db:     db,
		api:    api,
		sender: sender,
		as:     as,
		cfg:    cnfg,
	}
}

func (s *SubmitCommitTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var sectorParamsArr []struct {
		SpID         int64  `db:"sp_id"`
		SectorNumber int64  `db:"sector_number"`
		Proof        []byte `db:"porep_proof"`
	}

	err = s.db.Select(ctx, &sectorParamsArr, `
		SELECT sp_id, sector_number, porep_proof
		FROM sectors_sdr_pipeline
		WHERE task_id_commit_msg = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting sector params: %w", err)
	}

	if len(sectorParamsArr) != 1 {
		return false, xerrors.Errorf("expected 1 sector params, got %d", len(sectorParamsArr))
	}
	sectorParams := sectorParamsArr[0]

	var pieces []struct {
		PieceIndex int64           `db:"piece_index"`
		PieceCID   string          `db:"piece_cid"`
		PieceSize  int64           `db:"piece_size"`
		Proposal   json.RawMessage `db:"f05_deal_proposal"`
		Manifest   json.RawMessage `db:"direct_piece_activation_manifest"`
		DealID     abi.DealID      `db:"f05_deal_id"`
	}

	err = s.db.Select(ctx, &pieces, `
		SELECT piece_index,
		       piece_cid,
		       piece_size,
		       f05_deal_proposal,
		       direct_piece_activation_manifest,
		       COALESCE(f05_deal_id, 0) AS f05_deal_id
		FROM sectors_sdr_initial_pieces
		WHERE sp_id = $1 AND sector_number = $2 ORDER BY piece_index ASC`, sectorParams.SpID, sectorParams.SectorNumber)
	if err != nil {
		return false, xerrors.Errorf("getting pieces: %w", err)
	}

	maddr, err := address.NewIDAddress(uint64(sectorParams.SpID))
	if err != nil {
		return false, xerrors.Errorf("getting miner address: %w", err)
	}

	ts, err := s.api.ChainHead(ctx)
	if err != nil {
		return false, xerrors.Errorf("getting chain head: %w", err)
	}

	pci, err := s.api.StateSectorPreCommitInfo(ctx, maddr, abi.SectorNumber(sectorParams.SectorNumber), ts.Key())
	if err != nil {
		return false, xerrors.Errorf("getting precommit info: %w", err)
	}
	if pci == nil {
		return false, xerrors.Errorf("precommit info not found on chain")
	}

	mi, err := s.api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return false, xerrors.Errorf("getting miner info: %w", err)
	}

	params := miner.ProveCommitSectors3Params{
		RequireActivationSuccess:   s.cfg.RequireActivationSuccess,
		RequireNotificationSuccess: s.cfg.RequireNotificationSuccess,
	}

	var pams []miner.PieceActivationManifest

	for _, piece := range pieces {
		if piece.Proposal != nil {
			var prop *market.DealProposal
			err = json.Unmarshal(piece.Proposal, &prop)
			if err != nil {
				return false, xerrors.Errorf("marshalling json to deal proposal: %w", err)
			}
			alloc, err := s.api.StateGetAllocationIdForPendingDeal(ctx, piece.DealID, types.EmptyTSK)
			if err != nil {
				return false, xerrors.Errorf("getting allocation for deal %d: %w", piece.DealID, err)
			}
			clid, err := s.api.StateLookupID(ctx, prop.Client, types.EmptyTSK)
			if err != nil {
				return false, xerrors.Errorf("getting client address for deal %d: %w", piece.DealID, err)
			}

			clientId, err := address.IDFromAddress(clid)
			if err != nil {
				return false, xerrors.Errorf("getting client address for deal %d: %w", piece.DealID, err)
			}

			var vac *miner2.VerifiedAllocationKey
			if alloc != verifregtypes9.NoAllocationID {
				vac = &miner2.VerifiedAllocationKey{
					Client: abi.ActorID(clientId),
					ID:     verifreg13.AllocationId(alloc),
				}
			}

			payload, err := cborutil.Dump(piece.DealID)
			if err != nil {
				return false, xerrors.Errorf("serializing deal id: %w", err)
			}

			pams = append(pams, miner.PieceActivationManifest{
				CID:                   prop.PieceCID,
				Size:                  prop.PieceSize,
				VerifiedAllocationKey: vac,
				Notify: []miner2.DataActivationNotification{
					{
						Address: market.Address,
						Payload: payload,
					},
				},
			})
		} else {
			var pam *miner.PieceActivationManifest
			err = json.Unmarshal(piece.Manifest, &pam)
			if err != nil {
				return false, xerrors.Errorf("marshalling json to PieceManifest: %w", err)
			}
			err = s.allocationCheck(ctx, pam, pci, abi.ActorID(sectorParams.SpID), ts)
			if err != nil {
				return false, err
			}
			pams = append(pams, *pam)
		}
	}

	params.SectorActivations = append(params.SectorActivations, miner.SectorActivationManifest{
		SectorNumber: abi.SectorNumber(sectorParams.SectorNumber),
		Pieces:       pams,
	})
	params.SectorProofs = append(params.SectorProofs, sectorParams.Proof)

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return false, xerrors.Errorf("could not serialize commit params: %w", err)
	}

	collateral, err := s.api.StateMinerInitialPledgeCollateral(ctx, maddr, pci.Info, ts.Key())
	if err != nil {
		return false, xerrors.Errorf("getting initial pledge collateral: %w", err)
	}

	collateral = big.Sub(collateral, pci.PreCommitDeposit)
	if collateral.LessThan(big.Zero()) {
		collateral = big.Zero()
	}

	a, _, err := s.as.AddressFor(ctx, s.api, maddr, mi, api.CommitAddr, collateral, big.Zero())
	if err != nil {
		return false, xerrors.Errorf("getting address for precommit: %w", err)
	}

	msg := &types.Message{
		To:     maddr,
		From:   a,
		Method: builtin.MethodsMiner.ProveCommitSectors3,
		Params: enc.Bytes(),
		Value:  collateral, // todo config for pulling from miner balance!!
	}

	mss := &api.MessageSendSpec{
		MaxFee: abi.TokenAmount(s.cfg.maxFee),
	}

	mcid, err := s.sender.Send(ctx, msg, mss, "commit")
	if err != nil {
		return false, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	_, err = s.db.Exec(ctx, `UPDATE sectors_sdr_pipeline SET commit_msg_cid = $1, after_commit_msg = TRUE, task_id_commit_msg = NULL WHERE sp_id = $2 AND sector_number = $3`, mcid, sectorParams.SpID, sectorParams.SectorNumber)
	if err != nil {
		return false, xerrors.Errorf("updating commit_msg_cid: %w", err)
	}

	_, err = s.db.Exec(ctx, `INSERT INTO message_waits (signed_message_cid) VALUES ($1)`, mcid)
	if err != nil {
		return false, xerrors.Errorf("inserting into message_waits: %w", err)
	}

	if err := s.transferFinalizedSectorData(ctx, sectorParams.SpID, sectorParams.SectorNumber); err != nil {
		return false, xerrors.Errorf("transferring finalized sector data: %w", err)
	}

	return true, nil
}

func (s *SubmitCommitTask) transferFinalizedSectorData(ctx context.Context, spID, sectorNum int64) error {
	if _, err := s.db.Exec(ctx, `
        INSERT INTO sectors_meta (
            sp_id,
            sector_num,
            reg_seal_proof,
            ticket_epoch,
            ticket_value,
            orig_sealed_cid,
            orig_unsealed_cid,
            cur_sealed_cid,
            cur_unsealed_cid,
            msg_cid_precommit,
            msg_cid_commit,
            seed_epoch,
            seed_value
        )
        SELECT
            sp_id,
            sector_number as sector_num,
            reg_seal_proof,
            ticket_epoch,
            ticket_value,
            tree_r_cid as orig_sealed_cid,
            tree_d_cid as orig_unsealed_cid,
            tree_r_cid as cur_sealed_cid,
            tree_d_cid as cur_unsealed_cid,
            precommit_msg_cid,
            commit_msg_cid,
            seed_epoch,
            seed_value
        FROM
            sectors_sdr_pipeline
        WHERE
            sp_id = $1 AND
            sector_number = $2
        ON CONFLICT (sp_id, sector_num) DO UPDATE SET
            reg_seal_proof = excluded.reg_seal_proof,
            ticket_epoch = excluded.ticket_epoch,
            ticket_value = excluded.ticket_value,
            orig_sealed_cid = excluded.orig_sealed_cid,
            cur_sealed_cid = excluded.cur_sealed_cid,
            msg_cid_precommit = excluded.msg_cid_precommit,
            msg_cid_commit = excluded.msg_cid_commit,
            seed_epoch = excluded.seed_epoch,
            seed_value = excluded.seed_value;
    `, spID, sectorNum); err != nil {
		return fmt.Errorf("failed to insert/update sectors_meta: %w", err)
	}

	// Execute the query for piece metadata
	if _, err := s.db.Exec(ctx, `
        INSERT INTO sectors_meta_pieces (
            sp_id,
            sector_num,
            piece_num,
            piece_cid,
            piece_size,
            requested_keep_data,
            raw_data_size,
            start_epoch,
            orig_end_epoch,
            f05_deal_id,
            ddo_pam
        )
        SELECT
            sp_id,
            sector_number AS sector_num,
            piece_index AS piece_num,
            piece_cid,
            piece_size,
            not data_delete_on_finalize as requested_keep_data,
            data_raw_size,
            COALESCE(f05_deal_start_epoch, direct_start_epoch) as start_epoch,
            COALESCE(f05_deal_end_epoch, direct_end_epoch) as orig_end_epoch,
            f05_deal_id,
            direct_piece_activation_manifest as ddo_pam
        FROM
            sectors_sdr_initial_pieces
        WHERE
            sp_id = $1 AND
            sector_number = $2
        ON CONFLICT (sp_id, sector_num, piece_num) DO UPDATE SET
            piece_cid = excluded.piece_cid,
            piece_size = excluded.piece_size,
            requested_keep_data = excluded.requested_keep_data,
            raw_data_size = excluded.raw_data_size,
            start_epoch = excluded.start_epoch,
            orig_end_epoch = excluded.orig_end_epoch,
            f05_deal_id = excluded.f05_deal_id,
            ddo_pam = excluded.ddo_pam;
    `, spID, sectorNum); err != nil {
		return fmt.Errorf("failed to insert/update sector_meta_pieces: %w", err)
	}

	return nil
}

func (s *SubmitCommitTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	id := ids[0]
	return &id, nil
}

func (s *SubmitCommitTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  128,
		Name: "CommitSubmit",
		Cost: resources.Resources{
			Cpu: 0,
			Gpu: 0,
			Ram: 1 << 20,
		},
		MaxFailures: 16,
	}
}

func (s *SubmitCommitTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	s.sp.pollers[pollerCommitMsg].Set(taskFunc)
}

func (s *SubmitCommitTask) allocationCheck(ctx context.Context, piece *miner.PieceActivationManifest, precomitInfo *miner.SectorPreCommitOnChainInfo, miner abi.ActorID, ts *types.TipSet) error {
	// skip pieces not claiming an allocation
	if piece.VerifiedAllocationKey == nil {
		return nil
	}
	addr, err := address.NewIDAddress(uint64(piece.VerifiedAllocationKey.Client))
	if err != nil {
		return err
	}

	alloc, err := s.api.StateGetAllocation(ctx, addr, verifregtypes9.AllocationId(piece.VerifiedAllocationKey.ID), ts.Key())
	if err != nil {
		return err
	}
	if alloc == nil {
		return xerrors.Errorf("no allocation found for piece %s with allocation ID %d", piece.CID.String(), piece.VerifiedAllocationKey.ID)
	}
	if alloc.Provider != miner {
		return xerrors.Errorf("provider id mismatch for piece %s: expected %d and found %d", piece.CID.String(), miner, alloc.Provider)
	}
	if alloc.Size != piece.Size {
		return xerrors.Errorf("size mismatch for piece %s: expected %d and found %d", piece.CID.String(), piece.Size, alloc.Size)
	}
	if precomitInfo.Info.Expiration < ts.Height()+alloc.TermMin {
		return xerrors.Errorf("sector expiration %d is before than allocation TermMin %d for piece %s", precomitInfo.Info.Expiration, ts.Height()+alloc.TermMin, piece.CID.String())
	}
	if precomitInfo.Info.Expiration > ts.Height()+alloc.TermMax {
		return xerrors.Errorf("sector expiration %d is later than allocation TermMax %d for piece %s", precomitInfo.Info.Expiration, ts.Height()+alloc.TermMax, piece.CID.String())
	}

	return nil
}

var _ harmonytask.TaskInterface = &SubmitCommitTask{}
