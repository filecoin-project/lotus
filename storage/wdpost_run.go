package storage

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/filecoin-project/go-bitfield"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

var errNoPartitions = errors.New("no partitions")

func (s *WindowPoStScheduler) failPost(deadline *miner.DeadlineInfo) {
	log.Errorf("TODO")
	/*s.failLk.Lock()
	if eps > s.failed {
		s.failed = eps
	}
	s.failLk.Unlock()*/
}

func (s *WindowPoStScheduler) doPost(ctx context.Context, deadline *miner.DeadlineInfo, ts *types.TipSet) {
	ctx, abort := context.WithCancel(ctx)

	s.abort = abort
	s.activeDeadline = deadline

	go func() {
		defer abort()

		ctx, span := trace.StartSpan(ctx, "WindowPoStScheduler.doPost")
		defer span.End()

		proof, err := s.runPost(ctx, *deadline, ts)
		switch err {
		case errNoPartitions:
			return
		case nil:
			if err := s.submitPost(ctx, proof); err != nil {
				log.Errorf("submitPost failed: %+v", err)
				s.failPost(deadline)
				return
			}
		default:
			log.Errorf("runPost failed: %+v", err)
			s.failPost(deadline)
			return
		}
	}()
}

func (s *WindowPoStScheduler) checkRecoveries(ctx context.Context, deadline uint64, ts *types.TipSet) error {
	faults, err := s.api.StateMinerFaults(ctx, s.actor, ts.Key())
	if err != nil {
		return xerrors.Errorf("getting on-chain faults: %w", err)
	}

	fc, err := faults.Count()
	if err != nil {
		return xerrors.Errorf("counting faulty sectors: %w", err)
	}

	if fc == 0 {
		return nil
	}

	recov, err := s.api.StateMinerRecoveries(ctx, s.actor, ts.Key())
	if err != nil {
		return xerrors.Errorf("getting on-chain recoveries: %w", err)
	}

	unrecovered, err := bitfield.SubtractBitField(faults, recov)
	if err != nil {
		return xerrors.Errorf("subtracting recovered set from fault set: %w", err)
	}

	uc, err := unrecovered.Count()
	if err != nil {
		return xerrors.Errorf("counting unrecovered sectors: %w", err)
	}

	if uc == 0 {
		return nil
	}

	spt, err := s.proofType.RegisteredSealProof()
	if err != nil {
		return xerrors.Errorf("getting seal proof type: %w", err)
	}

	mid, err := address.IDFromAddress(s.actor)
	if err != nil {
		return err
	}

	sectors := make(map[abi.SectorID]struct{})
	var tocheck []abi.SectorID
	err = unrecovered.ForEach(func(snum uint64) error {
		s := abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: abi.SectorNumber(snum),
		}

		tocheck = append(tocheck, s)
		sectors[s] = struct{}{}
		return nil
	})
	if err != nil {
		return xerrors.Errorf("iterating over unrecovered bitfield: %w", err)
	}

	bad, err := s.faultTracker.CheckProvable(ctx, spt, tocheck)
	if err != nil {
		return xerrors.Errorf("checking provable sectors: %w", err)
	}
	for _, id := range bad {
		delete(sectors, id)
	}

	log.Warnw("Recoverable sectors", "faulty", len(tocheck), "recoverable", len(sectors))

	if len(sectors) == 0 { // nothing to recover
		return nil
	}

	sbf := bitfield.New()
	for s := range sectors {
		(&sbf).Set(uint64(s.Number))
	}

	params := &miner.DeclareFaultsRecoveredParams{
		Recoveries: []miner.RecoveryDeclaration{{Deadline: deadline, Sectors: &sbf}},
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return xerrors.Errorf("could not serialize declare recoveries parameters: %w", aerr)
	}

	msg := &types.Message{
		To:       s.actor,
		From:     s.worker,
		Method:   builtin.MethodsMiner.DeclareFaultsRecovered,
		Params:   enc,
		Value:    types.NewInt(0),
		GasLimit: 10000000, // i dont know help
		GasPrice: types.NewInt(2),
	}

	sm, err := s.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Warnw("declare faults recovered Message CID", "cid", sm.Cid())

	rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid(), build.MessageConfidence)
	if err != nil {
		return xerrors.Errorf("declare faults recovered wait error: %w", err)
	}

	if rec.Receipt.ExitCode != 0 {
		return xerrors.Errorf("declare faults recovered wait non-0 exit code: %d", rec.Receipt.ExitCode)
	}

	return nil
}

func (s *WindowPoStScheduler) checkFaults(ctx context.Context, ssi []abi.SectorNumber) ([]abi.SectorNumber, error) {
	//faults := s.prover.Scrub(ssi)
	log.Warnf("Stub checkFaults")

	/*declaredFaults := map[abi.SectorNumber]struct{}{}

	{
		chainFaults, err := s.api.StateMinerFaults(ctx, s.actor, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("checking on-chain faults: %w", err)
		}

		for _, fault := range chainFaults {
			declaredFaults[fault] = struct{}{}
		}
	}*/

	return nil, nil
}

// the input sectors must match with the miner actor
func (s *WindowPoStScheduler) getNeedProveSectors(ctx context.Context, deadlineSectors *abi.BitField, ts *types.TipSet) (*abi.BitField, error) {
	faults, err := s.api.StateMinerFaults(ctx, s.actor, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting on-chain faults: %w", err)
	}

	declaredFaults, err := bitfield.IntersectBitField(deadlineSectors, faults)
	if err != nil {
		return nil, xerrors.Errorf("failed to intersect proof sectors with faults: %w", err)
	}

	recoveries, err := s.api.StateMinerRecoveries(ctx, s.actor, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting on-chain recoveries: %w", err)
	}

	expectedRecoveries, err := bitfield.IntersectBitField(declaredFaults, recoveries)
	if err != nil {
		return nil, xerrors.Errorf("failed to intersect recoveries with faults: %w", err)
	}

	expectedFaults, err := bitfield.SubtractBitField(declaredFaults, expectedRecoveries)
	if err != nil {
		return nil, xerrors.Errorf("failed to subtract recoveries from faults: %w", err)
	}

	nonFaults, err := bitfield.SubtractBitField(deadlineSectors, expectedFaults)
	if err != nil {
		return nil, xerrors.Errorf("failed to diff bitfields: %w", err)
	}

	empty, err := nonFaults.IsEmpty()
	if err != nil {
		return nil, xerrors.Errorf("failed to check if bitfield was empty: %w", err)
	}
	if empty {
		return nil, xerrors.Errorf("no non-faulty sectors in partitions: %w", err)
	}

	return nonFaults, nil
}

func (s *WindowPoStScheduler) runPost(ctx context.Context, di miner.DeadlineInfo, ts *types.TipSet) (*miner.SubmitWindowedPoStParams, error) {
	ctx, span := trace.StartSpan(ctx, "storage.runPost")
	defer span.End()

	// check recoveries for the *next* deadline. It's already too late to
	// declare them for this deadline
	if err := s.checkRecoveries(ctx, (di.Index+1)%miner.WPoStPeriodDeadlines, ts); err != nil {
		// TODO: This is potentially quite bad, but not even trying to post when this fails is objectively worse
		log.Errorf("checking sector recoveries: %v", err)
	}

	buf := new(bytes.Buffer)
	if err := s.actor.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal address to cbor: %w", err)
	}
	rand, err := s.api.ChainGetRandomness(ctx, ts.Key(), crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to get chain randomness for windowPost (ts=%d; deadline=%d): %w", ts.Height(), di, err)
	}

	deadlines, err := s.api.StateMinerDeadlines(ctx, s.actor, ts.Key())
	if err != nil {
		return nil, err
	}

	firstPartition, _, err := miner.PartitionsForDeadline(deadlines, s.partitionSectors, di.Index)
	if err != nil {
		return nil, xerrors.Errorf("getting partitions for deadline: %w", err)
	}

	partitionCount, _, err := miner.DeadlineCount(deadlines, s.partitionSectors, di.Index)
	if err != nil {
		return nil, xerrors.Errorf("getting deadline partition count: %w", err)
	}

	dc, err := deadlines.Due[di.Index].Count()
	if err != nil {
		return nil, xerrors.Errorf("get deadline count: %w", err)
	}

	log.Infof("di: %+v", di)
	log.Infof("dc: %+v", dc)
	log.Infof("fp: %+v", firstPartition)
	log.Infof("pc: %+v", partitionCount)
	log.Infof("ts: %+v (%d)", ts.Key(), ts.Height())

	if partitionCount == 0 {
		return nil, errNoPartitions
	}

	partitions := make([]uint64, partitionCount)
	for i := range partitions {
		partitions[i] = firstPartition + uint64(i)
	}

	nps, err := s.getNeedProveSectors(ctx, deadlines.Due[di.Index], ts)
	if err != nil {
		return nil, xerrors.Errorf("get need prove sectors: %w", err)
	}

	ssi, err := s.sortedSectorInfo(ctx, nps, ts)
	if err != nil {
		return nil, xerrors.Errorf("getting sorted sector info: %w", err)
	}

	if len(ssi) == 0 {
		log.Warn("attempted to run windowPost without any sectors...")
		return nil, xerrors.Errorf("no sectors to run windowPost on")
	}

	log.Infow("running windowPost",
		"chain-random", rand,
		"deadline", di,
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
	log.Infow("submitting window PoSt", "elapsed", elapsed)

	return &miner.SubmitWindowedPoStParams{
		Deadline:   di.Index,
		Partitions: partitions,
		Proofs:     postOut,
		Skipped:    *abi.NewBitField(), // TODO: Faults here?
	}, nil
}

func (s *WindowPoStScheduler) sortedSectorInfo(ctx context.Context, deadlineSectors *abi.BitField, ts *types.TipSet) ([]abi.SectorInfo, error) {
	sset, err := s.api.StateMinerSectors(ctx, s.actor, deadlineSectors, false, ts.Key())
	if err != nil {
		return nil, err
	}

	sbsi := make([]abi.SectorInfo, len(sset))
	for k, sector := range sset {
		sbsi[k] = abi.SectorInfo{
			SectorNumber:    sector.ID,
			SealedCID:       sector.Info.Info.SealedCID,
			RegisteredProof: sector.Info.Info.RegisteredProof,
		}
	}

	return sbsi, nil
}

func (s *WindowPoStScheduler) submitPost(ctx context.Context, proof *miner.SubmitWindowedPoStParams) error {
	ctx, span := trace.StartSpan(ctx, "storage.commitPost")
	defer span.End()

	enc, aerr := actors.SerializeParams(proof)
	if aerr != nil {
		return xerrors.Errorf("could not serialize submit post parameters: %w", aerr)
	}

	msg := &types.Message{
		To:     s.actor,
		From:   s.worker,
		Method: builtin.MethodsMiner.SubmitWindowedPoSt,
		Params: enc,
		Value:  types.NewInt(1000), // currently hard-coded late fee in actor, returned if not late
		// TODO: Gaslimit needs to be calculated accurately. Before that, use the largest Gaslimit
		GasLimit: build.BlockGasLimit,
		GasPrice: types.NewInt(1),
	}

	// TODO: consider maybe caring about the output
	sm, err := s.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Infof("Submitted window post: %s", sm.Cid())

	go func() {
		rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid(), build.MessageConfidence)
		if err != nil {
			log.Error(err)
			return
		}

		if rec.Receipt.ExitCode == 0 {
			return
		}

		log.Errorf("Submitting window post %s failed: exit %d", sm.Cid(), rec.Receipt.ExitCode)
	}()

	return nil
}
