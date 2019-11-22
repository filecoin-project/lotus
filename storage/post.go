package storage

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

const postMsgTimeout = 20

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
		log.Warn("Not proving yet")
		return
	}

	m.postLk.Lock()
	if m.schedPost > 0 {
		log.Warnf("PoSts already running %d", m.schedPost)
		m.postLk.Unlock()
		return
	}

	// height needs to be +1, because otherwise we'd be trying to schedule PoSt
	// at current block height
	ppe, _ = actors.ProvingPeriodEnd(ppe, ts.Height()+1)
	m.schedPost = ppe

	m.postLk.Unlock()

	log.Infof("Scheduling post at height %d", ppe-build.PoStChallangeTime)
	err = m.events.ChainAt(m.computePost(m.schedPost), func(ctx context.Context, ts *types.TipSet) error { // Revert
		// TODO: Cancel post
		log.Errorf("TODO: Cancel PoSt, re-run")
		return nil
	}, PoStConfidence, ppe-build.PoStChallangeTime)
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
		log.Warnw("PoSt computation running behind chain", "headPPE", headPPE, "ppe", ppe)
		ppe = headPPE
	}

	m.postLk.Lock()
	if m.schedPost >= ppe {
		// this probably can't happen
		log.Errorw("PoSt already scheduled", "schedPost", m.schedPost, "ppe", ppe)
		m.postLk.Unlock()
		return
	}

	m.schedPost = ppe
	m.postLk.Unlock()

	log.Infow("scheduling PoSt", "post-height", ppe-build.PoStChallangeTime,
		"height", ts.Height(), "ppe", ppe, "proving-period", provingPeriod)
	err = m.events.ChainAt(m.computePost(ppe), func(ctx context.Context, ts *types.TipSet) error { // Revert
		// TODO: Cancel post
		log.Errorf("TODO: Cancel PoSt, re-run")
		return nil
	}, PoStConfidence, ppe-build.PoStChallangeTime)
	if err != nil {
		// TODO: This is BAD, figure something out
		log.Errorf("scheduling PoSt failed: %+v", err)
		return
	}
}

type post struct {
	m *Miner

	ppe uint64
	ts  *types.TipSet

	// prep
	sset []*api.ChainSectorInfo
	r    []byte

	// run
	proof []byte

	// commit
	msg  *types.Message
	smsg cid.Cid
}

func (p *post) doPost(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "storage.computePost")
	defer span.End()

	if err := p.preparePost(ctx); err != nil {
		return err
	}

	if err := p.runPost(ctx); err != nil {
		return err
	}

	if err := p.commitPost(ctx); err != nil {
		return err
	}

	if err := p.waitCommit(ctx); err != nil {
		return err
	}

	return nil
}

func (p *post) preparePost(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "storage.preparePost")
	defer span.End()
	log.Info("preparePost")

	sset, err := p.m.api.StateMinerProvingSet(ctx, p.m.maddr, p.ts)
	if err != nil {
		return xerrors.Errorf("failed to get proving set for miner: %w", err)
	}
	if len(sset) == 0 {
		log.Warn("empty proving set! (ts.H: %d)", p.ts.Height())
	}

	p.sset = sset

	// Compute how many blocks back we have to look from the given tipset for the PoSt challenge
	challengeLookback := int((int64(p.ts.Height()) - int64(p.ppe)) + int64(build.PoStChallangeTime) + int64(build.PoStRandomnessLookback))
	r, err := p.m.api.ChainGetRandomness(ctx, p.ts.Key(), nil, challengeLookback)
	if err != nil {
		return xerrors.Errorf("failed to get chain randomness for post (ts=%d; ppe=%d): %w", p.ts.Height(), p.ppe, err)
	}
	p.r = r

	return nil
}

func (p *post) sortedSectorInfo() sectorbuilder.SortedSectorInfo {
	sbsi := make([]sectorbuilder.SectorInfo, len(p.sset))
	for k, sector := range p.sset {
		var commR [sectorbuilder.CommLen]byte
		copy(commR[:], sector.CommR)

		sbsi[k] = sectorbuilder.SectorInfo{
			SectorID: sector.SectorID,
			CommR:    commR,
		}
	}

	return sectorbuilder.NewSortedSectorInfo(sbsi)
}

func (p *post) runPost(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "storage.runPost")
	defer span.End()

	log.Infow("running PoSt", "delayed-by",
		int64(p.ts.Height())-(int64(p.ppe)-int64(build.PoStChallangeTime)),
		"chain-random", p.r, "ppe", p.ppe, "height", p.ts.Height(), "sectors", len(p.sset))

	tsStart := time.Now()
	var faults []uint64 // TODO

	var seed [32]byte
	copy(seed[:], p.r)

	proof, err := p.m.sb.GeneratePoSt(p.sortedSectorInfo(), seed, faults)
	if err != nil {
		return xerrors.Errorf("running post failed: %w", err)
	}
	elapsed := time.Since(tsStart)

	p.proof = proof
	log.Infow("submitting PoSt", "pLen", len(proof), "elapsed", elapsed)

	return nil
}

func (p *post) commitPost(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "storage.commitPost")
	defer span.End()

	params := &actors.SubmitPoStParams{
		Proof:   p.proof,
		DoneSet: types.BitFieldFromSet(nil),
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return xerrors.Errorf("could not serialize submit post parameters: %w", aerr)
	}

	p.msg = &types.Message{
		To:       p.m.maddr,
		From:     p.m.worker,
		Method:   actors.MAMethods.SubmitPoSt,
		Params:   enc,
		Value:    types.NewInt(1000), // currently hard-coded late fee in actor, returned if not late
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	log.Info("mpush")

	smsg, err := p.m.api.MpoolPushMessage(ctx, p.msg)
	if err != nil {
		return xerrors.Errorf("pushing message to mpool: %w", err)
	}
	p.smsg = smsg.Cid()

	return nil
}

func (p *post) waitCommit(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "storage.waitPost")
	defer span.End()

	log.Infof("Waiting for post %s to appear on chain", p.smsg)

	err := p.m.events.CalledMsg(ctx, func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH uint64) (more bool, err error) {
		if rec.ExitCode != 0 {
			log.Warnf("SubmitPoSt EXIT: %d", rec.ExitCode)
		}

		log.Infof("Post made it on chain! (height=%d)", ts.Height())

		return false, nil
	}, func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("post message reverted")
		return nil
	}, 3, postMsgTimeout, p.msg)
	if err != nil {
		return err
	}

	return nil
}

func (m *Miner) computePost(ppe uint64) func(ctx context.Context, ts *types.TipSet, curH uint64) error {
	called := 0
	return func(ctx context.Context, ts *types.TipSet, curH uint64) error {
		called++
		if called > 1 {
			log.Errorw("BUG: computePost callback called again", "ppe", ppe,
				"height", ts.Height(), "curH", curH, "called", called-1)
			return nil
		}

		err := (&post{
			m:   m,
			ppe: ppe,
			ts:  ts,
		}).doPost(ctx)
		if err != nil {
			return xerrors.Errorf("doPost: %w", err)
		}

		m.scheduleNextPost(ppe + build.ProvingPeriodDuration)
		return nil
	}
}

func sectorIdList(si []*api.ChainSectorInfo) []uint64 {
	out := make([]uint64, len(si))
	for i, s := range si {
		out[i] = s.SectorID
	}
	return out
}
