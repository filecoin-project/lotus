package seal

import (
	"bytes"
	"context"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/curiosrc/message"
	"github.com/filecoin-project/lotus/curiosrc/multictladdr"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/storage/ctladdr"
)

type SubmitCommitAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (big.Int, error)
	StateSectorPreCommitInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error)
	ctladdr.NodeApi
}

type SubmitCommitTask struct {
	sp  *SealPoller
	db  *harmonydb.DB
	api SubmitCommitAPI

	sender *message.Sender
	as     *multictladdr.MultiAddressSelector

	maxFee types.FIL
}

func NewSubmitCommitTask(sp *SealPoller, db *harmonydb.DB, api SubmitCommitAPI, sender *message.Sender, as *multictladdr.MultiAddressSelector, maxFee types.FIL) *SubmitCommitTask {
	return &SubmitCommitTask{
		sp:     sp,
		db:     db,
		api:    api,
		sender: sender,
		as:     as,

		maxFee: maxFee,
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

	maddr, err := address.NewIDAddress(uint64(sectorParams.SpID))
	if err != nil {
		return false, xerrors.Errorf("getting miner address: %w", err)
	}

	params := miner.ProveCommitSectorParams{
		SectorNumber: abi.SectorNumber(sectorParams.SectorNumber),
		Proof:        sectorParams.Proof,
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return false, xerrors.Errorf("could not serialize commit params: %w", err)
	}

	ts, err := s.api.ChainHead(ctx)
	if err != nil {
		return false, xerrors.Errorf("getting chain head: %w", err)
	}

	mi, err := s.api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return false, xerrors.Errorf("getting miner info: %w", err)
	}

	pci, err := s.api.StateSectorPreCommitInfo(ctx, maddr, abi.SectorNumber(sectorParams.SectorNumber), ts.Key())
	if err != nil {
		return false, xerrors.Errorf("getting precommit info: %w", err)
	}
	if pci == nil {
		return false, xerrors.Errorf("precommit info not found on chain")
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
		Method: builtin.MethodsMiner.ProveCommitSector, // todo ddo provecommit3
		Params: enc.Bytes(),
		Value:  collateral, // todo config for pulling from miner balance!!
	}

	mss := &api.MessageSendSpec{
		MaxFee: abi.TokenAmount(s.maxFee),
	}

	mcid, err := s.sender.Send(ctx, msg, mss, "commit")
	if err != nil {
		return false, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	_, err = s.db.Exec(ctx, `UPDATE sectors_sdr_pipeline SET commit_msg_cid = $1, after_commit_msg = TRUE WHERE sp_id = $2 AND sector_number = $3`, mcid, sectorParams.SpID, sectorParams.SectorNumber)
	if err != nil {
		return false, xerrors.Errorf("updating commit_msg_cid: %w", err)
	}

	_, err = s.db.Exec(ctx, `INSERT INTO message_waits (signed_message_cid) VALUES ($1)`, mcid)
	if err != nil {
		return false, xerrors.Errorf("inserting into message_waits: %w", err)
	}

	if err := s.transferFinalizedSectorData(ctx, sectorParams.SpID, sectorParams.SectorNumber); err != nil {
		return false, xerrors.Errorf("transfering finalized sector data: %w", err)
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
            sector_number = $2 AND
            after_finalize = true
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
        INSERT INTO sector_meta_pieces (
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
            TRUE AS requested_keep_data,
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

var _ harmonytask.TaskInterface = &SubmitCommitTask{}
