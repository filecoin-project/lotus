package storage

import (
	"context"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	cid "github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
)

func (m *Miner) SealSector(ctx context.Context, sid uint64) error {
	log.Info("committing sector")

	ssize, err := m.SectorSize(ctx)
	if err != nil {
		return xerrors.Errorf("failed to check out own sector size: %w", err)
	}

	_ = ssize

	sinfo, err := m.secst.SectorStatus(sid)
	if err != nil {
		return xerrors.Errorf("failed to check status for sector %d: %w", sid, err)
	}

	params := &actors.SectorPreCommitInfo{
		CommD: sinfo.CommD[:],
		CommR: sinfo.CommR[:],
		Epoch: sinfo.Ticket.BlockHeight,

		//DealIDs:      deals,
		SectorNumber: sinfo.SectorID,
	}
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return errors.Wrap(aerr, "could not serialize commit sector parameters")
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.worker,
		Method:   actors.MAMethods.PreCommitSector,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return errors.Wrap(err, "pushing message to mpool")
	}

	go m.waitForPreCommitMessage(context.TODO(), sinfo.SectorID, smsg.Cid())

	// TODO: maybe return a wait channel?
	return nil
}

func (m *Miner) waitForPreCommitMessage(ctx context.Context, sid uint64, mcid cid.Cid) {
	// would be ideal to just use the events.Called handler, but it wouldnt be able to handle individual message timeouts
	mw, err := m.api.StateWaitMsg(ctx, mcid)
	if err != nil {
		return
	}

	randHeight := mw.TipSet.Height() + build.InteractivePoRepDelay

	err = m.events.ChainAt(func(ts *types.TipSet, curH uint64) error {
		return m.scheduleComputeProof(ctx, sid, ts, randHeight)
	}, func(ts *types.TipSet) error {
		log.Warn("revert in interactive commit sector step")
		return nil
	}, 3, mw.TipSet.Height()+build.InteractivePoRepDelay)
	if err != nil {
		log.Warn("waitForPreCommitMessage ChainAt errored: ", err)
	}
}

func (m *Miner) scheduleComputeProof(ctx context.Context, sid uint64, ts *types.TipSet, rheight uint64) error {
	go func() {
		rand, err := m.api.ChainGetRandomness(ctx, ts, nil, int(ts.Height()-rheight))
		if err != nil {
			log.Error(errors.Errorf("failed to get randomness for computing seal proof: %w", err))
			return
		}

		proof, err := m.secst.SealComputeProof(ctx, sid, rand)
		if err != nil {
			log.Error(errors.Errorf("computing seal proof failed: %w", err))
			return
		}

		params := &actors.SectorProveCommitInfo{
			Proof:    proof,
			SectorID: sid,
			//DealIDs:      deals,
		}

		_ = params
		enc, aerr := actors.SerializeParams(nil)
		if aerr != nil {
			log.Error(errors.Wrap(aerr, "could not serialize commit sector parameters"))
			return
		}

		msg := &types.Message{
			To:       m.maddr,
			From:     m.worker,
			Method:   actors.MAMethods.ProveCommitSector,
			Params:   enc,
			Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
			GasLimit: types.NewInt(1000000 /* i dont know help */),
			GasPrice: types.NewInt(1),
		}

		smsg, err := m.api.MpoolPushMessage(ctx, msg)
		if err != nil {
			log.Error(errors.Wrap(err, "pushing message to mpool"))
		}

		// TODO: now wait for this to get included and handle errors?
		mw, err := m.api.StateWaitMsg(ctx, smsg.Cid())
		if err != nil {
			log.Errorf("failed to wait for porep inclusion: %s", err)
			return
		}

		if mw.Receipt.ExitCode != 0 {
			log.Error("UNHANDLED: submitting sector proof failed")
			return
		}

		m.beginPosting(ctx)
	}()

	return nil
}
