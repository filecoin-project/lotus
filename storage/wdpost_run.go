package storage

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/filecoin-project/go-bitfield"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/journal"
)

var errNoPartitions = errors.New("no partitions")

func (s *WindowPoStScheduler) failPost(err error, deadline *miner.DeadlineInfo) {
	journal.J.RecordEvent(s.wdPoStEvtType, func() interface{} {
		return s.enrichWithTipset(WindowPoStEvt{
			State:    "failed",
			Deadline: s.activeDeadline,
			Error:    err,
		})
	})

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

	journal.J.RecordEvent(s.wdPoStEvtType, func() interface{} {
		return s.enrichWithTipset(WindowPoStEvt{
			State:    "started",
			Deadline: s.activeDeadline,
		})
	})

	go func() {
		defer abort()

		ctx, span := trace.StartSpan(ctx, "WindowPoStScheduler.doPost")
		defer span.End()

		// recordEvent records a successful proofs_processed event in the
		// journal, even if it was a noop (no partitions).
		recordEvent := func(partitions []miner.PoStPartition, mcid cid.Cid) {
			journal.J.RecordEvent(s.wdPoStEvtType, func() interface{} {
				return s.enrichWithTipset(WindowPoStEvt{
					State:    "proofs_processed",
					Deadline: s.activeDeadline,
					Proofs: &WindowPoStEvt_Proofs{
						Partitions: partitions,
						MessageCID: mcid,
					},
				})
			})
		}

		proof, err := s.runPost(ctx, *deadline, ts)
		switch err {
		case errNoPartitions:
			recordEvent(nil, cid.Undef)
			return
		case nil:
			sm, err := s.submitPost(ctx, proof)
			if err != nil {
				log.Errorf("submitPost failed: %+v", err)
				s.failPost(err, deadline)
				return
			}
			recordEvent(proof.Partitions, sm.Cid())
		default:
			log.Errorf("runPost failed: %+v", err)
			s.failPost(err, deadline)
			return
		}

		journal.J.RecordEvent(s.wdPoStEvtType, func() interface{} {
			return s.enrichWithTipset(WindowPoStEvt{
				State:    "succeeded",
				Deadline: s.activeDeadline,
			})
		})
	}()
}

func (s *WindowPoStScheduler) checkSectors(ctx context.Context, check abi.BitField) (abi.BitField, error) {
	spt, err := s.proofType.RegisteredSealProof()
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("getting seal proof type: %w", err)
	}

	mid, err := address.IDFromAddress(s.actor)
	if err != nil {
		return bitfield.BitField{}, err
	}

	sectors := make(map[abi.SectorID]struct{})
	var tocheck []abi.SectorID
	err = check.ForEach(func(snum uint64) error {
		s := abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: abi.SectorNumber(snum),
		}

		tocheck = append(tocheck, s)
		sectors[s] = struct{}{}
		return nil
	})
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("iterating over bitfield: %w", err)
	}

	bad, err := s.faultTracker.CheckProvable(ctx, spt, tocheck)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("checking provable sectors: %w", err)
	}
	for _, id := range bad {
		delete(sectors, id)
	}

	log.Warnw("Checked sectors", "checked", len(tocheck), "good", len(sectors))

	sbf := bitfield.New()
	for s := range sectors {
		sbf.Set(uint64(s.Number))
	}

	return sbf, nil
}

func (s *WindowPoStScheduler) checkNextRecoveries(ctx context.Context, dlIdx uint64, partitions []*miner.Partition) ([]miner.RecoveryDeclaration, *types.SignedMessage, error) {
	ctx, span := trace.StartSpan(ctx, "storage.checkNextRecoveries")
	defer span.End()

	faulty := uint64(0)
	params := &miner.DeclareFaultsRecoveredParams{
		Recoveries: []miner.RecoveryDeclaration{},
	}

	for partIdx, partition := range partitions {
		unrecovered, err := bitfield.SubtractBitField(partition.Faults, partition.Recoveries)
		if err != nil {
			return nil, nil, xerrors.Errorf("subtracting recovered set from fault set: %w", err)
		}

		uc, err := unrecovered.Count()
		if err != nil {
			return nil, nil, xerrors.Errorf("counting unrecovered sectors: %w", err)
		}

		if uc == 0 {
			continue
		}

		faulty += uc

		recovered, err := s.checkSectors(ctx, unrecovered)
		if err != nil {
			return nil, nil, xerrors.Errorf("checking unrecovered sectors: %w", err)
		}

		// if all sectors failed to recover, don't declare recoveries
		recoveredCount, err := recovered.Count()
		if err != nil {
			return nil, nil, xerrors.Errorf("counting recovered sectors: %w", err)
		}

		if recoveredCount == 0 {
			continue
		}

		params.Recoveries = append(params.Recoveries, miner.RecoveryDeclaration{
			Deadline:  dlIdx,
			Partition: uint64(partIdx),
			Sectors:   recovered,
		})
	}

	recoveries := params.Recoveries
	if len(recoveries) == 0 {
		if faulty != 0 {
			log.Warnw("No recoveries to declare", "deadline", dlIdx, "faulty", faulty)
		}

		return recoveries, nil, nil
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return recoveries, nil, xerrors.Errorf("could not serialize declare recoveries parameters: %w", aerr)
	}

	msg := &types.Message{
		To:     s.actor,
		From:   s.worker,
		Method: builtin.MethodsMiner.DeclareFaultsRecovered,
		Params: enc,
		Value:  types.NewInt(0),
	}
	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)}
	s.setSender(ctx, msg, spec)

	sm, err := s.api.MpoolPushMessage(ctx, msg, &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)})
	if err != nil {
		return recoveries, sm, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Warnw("declare faults recovered Message CID", "cid", sm.Cid())

	rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid(), build.MessageConfidence)
	if err != nil {
		return recoveries, sm, xerrors.Errorf("declare faults recovered wait error: %w", err)
	}

	if rec.Receipt.ExitCode != 0 {
		return recoveries, sm, xerrors.Errorf("declare faults recovered wait non-0 exit code: %d", rec.Receipt.ExitCode)
	}

	return recoveries, sm, nil
}

func (s *WindowPoStScheduler) checkNextFaults(ctx context.Context, dlIdx uint64, partitions []*miner.Partition) ([]miner.FaultDeclaration, *types.SignedMessage, error) {
	ctx, span := trace.StartSpan(ctx, "storage.checkNextFaults")
	defer span.End()

	bad := uint64(0)
	params := &miner.DeclareFaultsParams{
		Faults: []miner.FaultDeclaration{},
	}

	for partIdx, partition := range partitions {
		toCheck, err := partition.ActiveSectors()
		if err != nil {
			return nil, nil, xerrors.Errorf("getting active sectors: %w", err)
		}

		good, err := s.checkSectors(ctx, toCheck)
		if err != nil {
			return nil, nil, xerrors.Errorf("checking sectors: %w", err)
		}

		faulty, err := bitfield.SubtractBitField(toCheck, good)
		if err != nil {
			return nil, nil, xerrors.Errorf("calculating faulty sector set: %w", err)
		}

		c, err := faulty.Count()
		if err != nil {
			return nil, nil, xerrors.Errorf("counting faulty sectors: %w", err)
		}

		if c == 0 {
			continue
		}

		bad += c

		params.Faults = append(params.Faults, miner.FaultDeclaration{
			Deadline:  dlIdx,
			Partition: uint64(partIdx),
			Sectors:   faulty,
		})
	}

	faults := params.Faults
	if len(faults) == 0 {
		return faults, nil, nil
	}

	log.Errorw("DETECTED FAULTY SECTORS, declaring faults", "count", bad)

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return faults, nil, xerrors.Errorf("could not serialize declare faults parameters: %w", aerr)
	}

	msg := &types.Message{
		To:     s.actor,
		From:   s.worker,
		Method: builtin.MethodsMiner.DeclareFaults,
		Params: enc,
		Value:  types.NewInt(0), // TODO: Is there a fee?
	}
	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)}
	s.setSender(ctx, msg, spec)

	sm, err := s.api.MpoolPushMessage(ctx, msg, spec)
	if err != nil {
		return faults, sm, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Warnw("declare faults Message CID", "cid", sm.Cid())

	rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid(), build.MessageConfidence)
	if err != nil {
		return faults, sm, xerrors.Errorf("declare faults wait error: %w", err)
	}

	if rec.Receipt.ExitCode != 0 {
		return faults, sm, xerrors.Errorf("declare faults wait non-0 exit code: %d", rec.Receipt.ExitCode)
	}

	return faults, sm, nil
}

func (s *WindowPoStScheduler) runPost(ctx context.Context, di miner.DeadlineInfo, ts *types.TipSet) (*miner.SubmitWindowedPoStParams, error) {
	ctx, span := trace.StartSpan(ctx, "storage.runPost")
	defer span.End()

	go func() {
		// TODO: extract from runPost, run on fault cutoff boundaries

		// check faults / recoveries for the *next* deadline. It's already too
		// late to declare them for this deadline
		declDeadline := (di.Index + 2) % miner.WPoStPeriodDeadlines

		partitions, err := s.api.StateMinerPartitions(context.TODO(), s.actor, declDeadline, ts.Key())
		if err != nil {
			log.Errorf("getting partitions: %v", err)
			return
		}

		var (
			sigmsg     *types.SignedMessage
			recoveries []miner.RecoveryDeclaration
			faults     []miner.FaultDeclaration

			// optionalCid returns the CID of the message, or cid.Undef is the
			// message is nil. We don't need the argument (could capture the
			// pointer), but it's clearer and purer like that.
			optionalCid = func(sigmsg *types.SignedMessage) cid.Cid {
				if sigmsg == nil {
					return cid.Undef
				}
				return sigmsg.Cid()
			}
		)

		if recoveries, sigmsg, err = s.checkNextRecoveries(context.TODO(), declDeadline, partitions); err != nil {
			// TODO: This is potentially quite bad, but not even trying to post when this fails is objectively worse
			log.Errorf("checking sector recoveries: %v", err)
		}

		journal.J.RecordEvent(s.wdPoStEvtType, func() interface{} {
			return s.enrichWithTipset(WindowPoStEvt{
				State:    "recoveries_processed",
				Deadline: s.activeDeadline,
				Recoveries: &WindowPoStEvt_Recoveries{
					Declarations: recoveries,
					MessageCID:   optionalCid(sigmsg),
				},
			})
		})

		if faults, sigmsg, err = s.checkNextFaults(context.TODO(), declDeadline, partitions); err != nil {
			// TODO: This is also potentially really bad, but we try to post anyways
			log.Errorf("checking sector faults: %v", err)
		}

		journal.J.RecordEvent(s.wdPoStEvtType, func() interface{} {
			return s.enrichWithTipset(WindowPoStEvt{
				State:    "faults_processed",
				Deadline: s.activeDeadline,
				Faults: &WindowPoStEvt_Faults{
					Declarations: faults,
					MessageCID:   optionalCid(sigmsg),
				},
			})
		})
	}()

	buf := new(bytes.Buffer)
	if err := s.actor.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal address to cbor: %w", err)
	}

	rand, err := s.api.ChainGetRandomnessFromBeacon(ctx, ts.Key(), crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to get chain randomness for windowPost (ts=%d; deadline=%d): %w", ts.Height(), di, err)
	}

	partitions, err := s.api.StateMinerPartitions(ctx, s.actor, di.Index, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting partitions: %w", err)
	}

	params := &miner.SubmitWindowedPoStParams{
		Deadline:   di.Index,
		Partitions: make([]miner.PoStPartition, 0, len(partitions)),
		Proofs:     nil,
	}

	var sinfos []abi.SectorInfo
	sidToPart := map[abi.SectorNumber]uint64{}
	skipCount := uint64(0)

	for partIdx, partition := range partitions {
		// TODO: Can do this in parallel
		toProve, err := partition.ActiveSectors()
		if err != nil {
			return nil, xerrors.Errorf("getting active sectors: %w", err)
		}

		toProve, err = bitfield.MergeBitFields(toProve, partition.Recoveries)
		if err != nil {
			return nil, xerrors.Errorf("adding recoveries to set of sectors to prove: %w", err)
		}

		good, err := s.checkSectors(ctx, toProve)
		if err != nil {
			return nil, xerrors.Errorf("checking sectors to skip: %w", err)
		}

		skipped, err := bitfield.SubtractBitField(toProve, good)
		if err != nil {
			return nil, xerrors.Errorf("toProve - good: %w", err)
		}

		sc, err := skipped.Count()
		if err != nil {
			return nil, xerrors.Errorf("getting skipped sector count: %w", err)
		}

		skipCount += sc

		ssi, err := s.sectorInfo(ctx, good, ts)
		if err != nil {
			return nil, xerrors.Errorf("getting sorted sector info: %w", err)
		}

		if len(ssi) == 0 {
			continue
		}

		sinfos = append(sinfos, ssi...)
		for _, si := range ssi {
			sidToPart[si.SectorNumber] = uint64(partIdx)
		}

		params.Partitions = append(params.Partitions, miner.PoStPartition{
			Index:   uint64(partIdx),
			Skipped: skipped,
		})
	}

	if len(sinfos) == 0 {
		// nothing to prove..
		return nil, errNoPartitions
	}

	log.Infow("running windowPost",
		"chain-random", rand,
		"deadline", di,
		"height", ts.Height(),
		"skipped", skipCount)

	tsStart := build.Clock.Now()

	log.Infow("generating windowPost", "sectors", len(sinfos))

	mid, err := address.IDFromAddress(s.actor)
	if err != nil {
		return nil, err
	}

	postOut, postSkipped, err := s.prover.GenerateWindowPoSt(ctx, abi.ActorID(mid), sinfos, abi.PoStRandomness(rand))
	if err != nil {
		return nil, xerrors.Errorf("running post failed: %w", err)
	}

	if len(postOut) == 0 {
		return nil, xerrors.Errorf("received proofs back from generate window post")
	}

	params.Proofs = postOut

	for _, sector := range postSkipped {
		params.Partitions[sidToPart[sector.Number]].Skipped.Set(uint64(sector.Number))
	}

	elapsed := time.Since(tsStart)

	commEpoch := di.Open
	commRand, err := s.api.ChainGetRandomnessFromTickets(ctx, ts.Key(), crypto.DomainSeparationTag_PoStChainCommit, commEpoch, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to get chain randomness for windowPost (ts=%d; deadline=%d): %w", ts.Height(), di, err)
	}
	params.ChainCommitEpoch = commEpoch
	params.ChainCommitRand = commRand

	log.Infow("submitting window PoSt", "elapsed", elapsed)

	return params, nil
}

func (s *WindowPoStScheduler) sectorInfo(ctx context.Context, deadlineSectors abi.BitField, ts *types.TipSet) ([]abi.SectorInfo, error) {
	sset, err := s.api.StateMinerSectors(ctx, s.actor, &deadlineSectors, false, ts.Key())
	if err != nil {
		return nil, err
	}

	sbsi := make([]abi.SectorInfo, len(sset))
	for k, sector := range sset {
		sbsi[k] = abi.SectorInfo{
			SectorNumber: sector.ID,
			SealedCID:    sector.Info.SealedCID,
			SealProof:    sector.Info.SealProof,
		}
	}

	return sbsi, nil
}

func (s *WindowPoStScheduler) submitPost(ctx context.Context, proof *miner.SubmitWindowedPoStParams) (*types.SignedMessage, error) {
	ctx, span := trace.StartSpan(ctx, "storage.commitPost")
	defer span.End()

	var sm *types.SignedMessage

	enc, aerr := actors.SerializeParams(proof)
	if aerr != nil {
		return nil, xerrors.Errorf("could not serialize submit post parameters: %w", aerr)
	}

	msg := &types.Message{
		To:     s.actor,
		From:   s.worker,
		Method: builtin.MethodsMiner.SubmitWindowedPoSt,
		Params: enc,
		Value:  types.NewInt(1000), // currently hard-coded late fee in actor, returned if not late
	}
	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)}
	s.setSender(ctx, msg, spec)

	// TODO: consider maybe caring about the output
	sm, err := s.api.MpoolPushMessage(ctx, msg, spec)

	if err != nil {
		return nil, xerrors.Errorf("pushing message to mpool: %w", err)
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

	return sm, nil
}

func (s *WindowPoStScheduler) setSender(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) {
	mi, err := s.api.StateMinerInfo(ctx, s.actor, types.EmptyTSK)
	if err != nil {
		log.Errorw("error getting miner info", "error", err)

		// better than just failing
		msg.From = s.worker
		return
	}

	gm, err := s.api.GasEstimateMessageGas(ctx, msg, spec, types.EmptyTSK)
	if err != nil {
		log.Errorw("estimating gas", "error", err)
		msg.From = s.worker
		return
	}
	*msg = *gm

	minFunds := big.Add(msg.RequiredFunds(), msg.Value)

	pa, err := AddressFor(ctx, s.api, mi, PoStAddr, minFunds)
	if err != nil {
		log.Errorw("error selecting address for post", "error", err)
		msg.From = s.worker
		return
	}

	msg.From = pa
}
