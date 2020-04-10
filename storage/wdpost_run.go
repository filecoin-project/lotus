package storage

import (
	"bytes"
	"context"
	"github.com/filecoin-project/go-address"
	"time"

	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func (s *WindowPoStScheduler) failPost(deadline *Deadline) {
	log.Errorf("TODO")
	/*s.failLk.Lock()
	if eps > s.failed {
		s.failed = eps
	}
	s.failLk.Unlock()*/
}

func (s *WindowPoStScheduler) doPost(ctx context.Context, deadline *Deadline, ts *types.TipSet) {
	ctx, abort := context.WithCancel(ctx)

	s.abort = abort
	s.activeDeadline = deadline

	go func() {
		defer abort()

		ctx, span := trace.StartSpan(ctx, "WindowPoStScheduler.doPost")
		defer span.End()

		proof, err := s.runPost(ctx, *deadline, ts)
		if err != nil {
			log.Errorf("runPost failed: %+v", err)
			s.failPost(deadline)
			return
		}

		if err := s.submitPost(ctx, proof); err != nil {
			log.Errorf("submitPost failed: %+v", err)
			s.failPost(deadline)
			return
		}

	}()
}

func (s *WindowPoStScheduler) declareFaults(ctx context.Context, fc uint64, params *miner.DeclareTemporaryFaultsParams) error {
	log.Warnf("DECLARING %d FAULTS", fc)

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return xerrors.Errorf("could not serialize declare faults parameters: %w", aerr)
	}

	msg := &types.Message{
		To:       s.actor,
		From:     s.worker,
		Method:   builtin.MethodsMiner.DeclareTemporaryFaults,
		Params:   enc,
		Value:    types.NewInt(0),
		GasLimit: 10000000, // i dont know help
		GasPrice: types.NewInt(1),
	}

	sm, err := s.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return xerrors.Errorf("pushing faults message to mpool: %w", err)
	}

	rec, err := s.api.StateWaitMsg(ctx, sm.Cid())
	if err != nil {
		return xerrors.Errorf("waiting for declare faults: %w", err)
	}

	if rec.Receipt.ExitCode != 0 {
		return xerrors.Errorf("declare faults exit %d", rec.Receipt.ExitCode)
	}

	log.Infof("Faults declared successfully")
	return nil
}

func (s *WindowPoStScheduler) checkFaults(ctx context.Context, ssi []abi.SectorNumber) ([]abi.SectorNumber, error) {
	//faults := s.prover.Scrub(ssi)
	log.Warnf("Stub checkFaults")
	var faults []struct {
		SectorNum abi.SectorNumber
		Err       error
	}

	declaredFaults := map[abi.SectorNumber]struct{}{}

	{
		chainFaults, err := s.api.StateMinerFaults(ctx, s.actor, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("checking on-chain faults: %w", err)
		}

		for _, fault := range chainFaults {
			declaredFaults[fault] = struct{}{}
		}
	}

	var faultIDs []abi.SectorNumber
	if len(faults) > 0 {
		params := &miner.DeclareTemporaryFaultsParams{
			Duration:      900, // TODO: duration is annoying
			SectorNumbers: abi.NewBitField(),
		}

		for _, fault := range faults {
			if _, ok := declaredFaults[(fault.SectorNum)]; ok {
				continue
			}

			log.Warnf("new fault detected: sector %d: %s", fault.SectorNum, fault.Err)
			declaredFaults[fault.SectorNum] = struct{}{}
		}

		faultIDs = make([]abi.SectorNumber, 0, len(declaredFaults))
		for fault := range declaredFaults {
			faultIDs = append(faultIDs, fault)
			params.SectorNumbers.Set(uint64(fault))
		}

		if len(faultIDs) > 0 {
			if err := s.declareFaults(ctx, uint64(len(faultIDs)), params); err != nil {
				return nil, err
			}
		}
	}

	return faultIDs, nil
}

func (s *WindowPoStScheduler) runPost(ctx context.Context, deadline Deadline, ts *types.TipSet) (*abi.OnChainWindowPoStVerifyInfo, error) {
	ctx, span := trace.StartSpan(ctx, "storage.runPost")
	defer span.End()

	challengeRound := deadline.start // TODO: check with spec

	buf := new(bytes.Buffer)
	if err := s.actor.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal address to cbor: %w", err)
	}
	rand, err := s.api.ChainGetRandomness(ctx, ts.Key(), crypto.DomainSeparationTag_WindowedPoStChallengeSeed, challengeRound, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to get chain randomness for windowPost (ts=%d; deadline=%d): %w", ts.Height(), deadline, err)
	}

	partitions, err := s.getDeadlinePartitions(ts, deadline)
	if err != nil {
		return nil, err
	}

	ssi, err := s.sortedSectorInfo(ctx, partitions, ts)
	if err != nil {
		return nil, xerrors.Errorf("getting sorted sector info: %w", err)
	}
	if len(ssi) == 0 {
		log.Warn("attempted to run windowPost without any sectors...")
		return nil, xerrors.Errorf("no sectors to run windowPost on")
	}

	log.Infow("running windowPost",
		"chain-random", rand,
		"deadline", deadline,
		"height", ts.Height())

	var snums []abi.SectorNumber
	for _, si := range ssi {
		snums = append(snums, si.SectorNumber)
	}

	faults, err := s.checkFaults(ctx, snums)
	if err != nil {
		log.Errorf("Failed to declare faults: %+v", err)
	}

	tsStart := time.Now()

	log.Infow("generating windowPost",
		"sectors", len(ssi),
		"faults", len(faults))

	mid, err := address.IDFromAddress(s.actor)
	if err != nil {
		return nil, err
	}

	// TODO: Faults!
	postOut, err := s.prover.GenerateWindowPoSt(ctx, abi.ActorID(mid), ssi, abi.PoStRandomness(rand))
	if err != nil {
		return nil, xerrors.Errorf("running post failed: %w", err)
	}

	if len(postOut) == 0 {
		return nil, xerrors.Errorf("received proofs back from generate window post")
	}

	elapsed := time.Since(tsStart)
	log.Infow("submitting PoSt", "elapsed", elapsed)

	return &abi.OnChainWindowPoStVerifyInfo{
		Proofs: postOut,
	}, nil
}

func (s *WindowPoStScheduler) sortedSectorInfo(ctx context.Context, partitions []abiPartition, ts *types.TipSet) ([]abi.SectorInfo, error) {
	sset, err := s.getPartitionSectors(ts, partitions)
	if err != nil {
		return nil, err
	}

	sbsi := make([]abi.SectorInfo, len(sset))
	for k, sector := range sset {
		sbsi[k] = abi.SectorInfo{
			SectorNumber:    sector.SectorNumber,
			SealedCID:       sector.SealedCID,
			RegisteredProof: sector.RegisteredProof,
		}
	}

	return sbsi, nil
}

func (s *WindowPoStScheduler) submitPost(ctx context.Context, proof *abi.OnChainWindowPoStVerifyInfo) error {
	ctx, span := trace.StartSpan(ctx, "storage.commitPost")
	defer span.End()

	enc, aerr := actors.SerializeParams(proof)
	if aerr != nil {
		return xerrors.Errorf("could not serialize submit post parameters: %w", aerr)
	}

	msg := &types.Message{
		To:       s.actor,
		From:     s.worker,
		Method:   builtin.MethodsMiner.SubmitWindowedPoSt,
		Params:   enc,
		Value:    types.NewInt(1000), // currently hard-coded late fee in actor, returned if not late
		GasLimit: 10000000,           // i dont know help
		GasPrice: types.NewInt(1),
	}

	// TODO: consider maybe caring about the output
	sm, err := s.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Infof("Submitted fallback post: %s", sm.Cid())

	go func() {
		rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid())
		if err != nil {
			log.Error(err)
			return
		}

		if rec.Receipt.ExitCode == 0 {
			return
		}

		log.Errorf("Submitting fallback post %s failed: exit %d", sm.Cid(), rec.Receipt.ExitCode)
	}()

	return nil
}
