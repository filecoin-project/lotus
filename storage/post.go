package storage

import (
	"context"
	"encoding/base64"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/types"
)

func (m *Miner) beginPosting(ctx context.Context) {
	ts, err := m.api.ChainHead(context.TODO())
	if err != nil {
		log.Error(err)
		return
	}

	ppe, err := m.api.StateMinerProvingPeriodEnd(ctx, m.maddr, ts)
	if err != nil {
		log.Errorf("failed to get proving period end for miner: %s", err)
		return
	}

	if ppe == 0 {
		log.Errorf("Proving period end == 0")
		return
	}

	m.schedLk.Lock()
	if m.schedPost > 0 {
		log.Warnf("PoSts already running %d", m.schedPost)
		m.schedLk.Unlock()
		return
	}

	m.schedPost, _ = actors.ProvingPeriodEnd(ppe, ts.Height())

	m.schedLk.Unlock()

	log.Infof("Scheduling post at height %d", ppe-build.PoSTChallangeTime)
	err = m.events.ChainAt(m.computePost(m.schedPost), func(ts *types.TipSet) error { // Revert
		// TODO: Cancel post
		log.Errorf("TODO: Cancel PoSt, re-run")
		return nil
	}, PoStConfidence, ppe-build.PoSTChallangeTime)
	if err != nil {
		// TODO: This is BAD, figure something out
		log.Errorf("scheduling PoSt failed: %s", err)
		return
	}
}

func (m *Miner) scheduleNextPost(ppe uint64) {
	ts, err := m.api.ChainHead(context.TODO())
	if err != nil {
		log.Error(err)
		// TODO: retry
		return
	}

	headPPE, provingPeriod := actors.ProvingPeriodEnd(ppe, ts.Height())
	if headPPE > ppe {
		log.Warn("PoSt computation running behind chain")
		ppe = headPPE
	}

	m.schedLk.Lock()
	if m.schedPost >= ppe {
		// this probably can't happen
		log.Error("PoSt already scheduled: %d >= %d", m.schedPost, ppe)
		m.schedLk.Unlock()
		return
	}

	m.schedPost = ppe
	m.schedLk.Unlock()

	log.Infof("Scheduling post at height %d (head=%d; ppe=%d, period=%d)", ppe-build.PoSTChallangeTime, ts.Height(), ppe, provingPeriod)
	err = m.events.ChainAt(m.computePost(ppe), func(ts *types.TipSet) error { // Revert
		// TODO: Cancel post
		log.Errorf("TODO: Cancel PoSt, re-run")
		return nil
	}, PoStConfidence, ppe-build.PoSTChallangeTime)
	if err != nil {
		// TODO: This is BAD, figure something out
		log.Errorf("scheduling PoSt failed: %s", err)
		return
	}
}

func (m *Miner) computePost(ppe uint64) func(ts *types.TipSet, curH uint64) error {
	return func(ts *types.TipSet, curH uint64) error {

		ctx := context.TODO()

		sset, err := m.api.StateMinerProvingSet(ctx, m.maddr, ts)
		if err != nil {
			return xerrors.Errorf("failed to get proving set for miner: %w", err)
		}

		r, err := m.api.ChainGetRandomness(ctx, ts, nil, int(int64(ts.Height())-int64(ppe)+int64(build.PoSTChallangeTime))) // TODO: review: check math
		if err != nil {
			return xerrors.Errorf("failed to get chain randomness for post (ts=%d; ppe=%d): %w", ts.Height(), ppe, err)
		}

		log.Infof("running PoSt computation, rh=%d r=%s, ppe=%d, h=%d", int64(ts.Height())-int64(ppe)+int64(build.PoSTChallangeTime), base64.StdEncoding.EncodeToString(r), ppe, ts.Height())
		var faults []uint64
		proof, err := m.secst.RunPoSt(ctx, sset, r, faults)
		if err != nil {
			return xerrors.Errorf("running post failed: %w", err)
		}

		log.Infof("submitting PoSt pLen=%d", len(proof))

		params := &actors.SubmitPoStParams{
			Proof:   proof,
			DoneSet: types.BitFieldFromSet(sectorIdList(sset)),
		}

		enc, aerr := actors.SerializeParams(params)
		if aerr != nil {
			return xerrors.Errorf("could not serialize submit post parameters: %w", err)
		}

		msg := &types.Message{
			To:       m.maddr,
			From:     m.worker,
			Method:   actors.MAMethods.SubmitPoSt,
			Params:   enc,
			Value:    types.NewInt(1000), // currently hard-coded late fee in actor, returned if not late
			GasLimit: types.NewInt(100000 /* i dont know help */),
			GasPrice: types.NewInt(1),
		}

		log.Info("mpush")

		smsg, err := m.api.MpoolPushMessage(ctx, msg)
		if err != nil {
			return xerrors.Errorf("pushing message to mpool: %w", err)
		}

		log.Infof("Waiting for post %s to appear on chain", smsg.Cid())

		// make sure it succeeds...
		rec, err := m.api.ChainWaitMsg(ctx, smsg.Cid())
		if err != nil {
			return err
		}
		if rec.Receipt.ExitCode != 0 {
			log.Warnf("SubmitPoSt EXIT: %d", rec.Receipt.ExitCode)
			// TODO: Do something
		}

		m.scheduleNextPost(ppe + build.ProvingPeriodDuration)
		return nil
	}
}

func sectorIdList(si []*api.SectorInfo) []uint64 {
	out := make([]uint64, len(si))
	for i, s := range si {
		out[i] = s.SectorID
	}
	return out
}
