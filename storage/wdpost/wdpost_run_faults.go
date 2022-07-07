package wdpost

import (
	"context"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

// declareRecoveries identifies sectors that were previously marked as faulty
// for our miner, but are now recovered (i.e. are now provable again) and
// still not reported as such.
//
// It then reports the recovery on chain via a `DeclareFaultsRecovered`
// message to our miner actor.
//
// This is always invoked ahead of time, before the deadline for the evaluated
// sectors arrives. That way, recoveries are declared in preparation for those
// sectors to be proven.
//
// If a declaration is made, it awaits for build.MessageConfidence confirmations
// on chain before returning.
//
// TODO: the waiting should happen in the background. Right now this
//  is blocking/delaying the actual generation and submission of WindowPoSts in
//  this deadline!
func (s *WindowPoStScheduler) declareRecoveries(ctx context.Context, dlIdx uint64, partitions []api.Partition, tsk types.TipSetKey) ([][]miner.RecoveryDeclaration, []*types.SignedMessage, error) {
	ctx, span := trace.StartSpan(ctx, "storage.declareRecoveries")
	defer span.End()

	faulty := uint64(0)

	var batchedRecoveryDecls [][]miner.RecoveryDeclaration
	batchedRecoveryDecls = append(batchedRecoveryDecls, []miner.RecoveryDeclaration{})
	totalRecoveries := 0

	for partIdx, partition := range partitions {
		unrecovered, err := bitfield.SubtractBitField(partition.FaultySectors, partition.RecoveringSectors)
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

		recovered, err := s.checkSectors(ctx, unrecovered, tsk)
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

		// respect user config if set
		if s.maxPartitionsPerRecoveryMessage > 0 &&
			len(batchedRecoveryDecls[len(batchedRecoveryDecls)-1]) >= s.maxPartitionsPerRecoveryMessage {
			batchedRecoveryDecls = append(batchedRecoveryDecls, []miner.RecoveryDeclaration{})
		}

		batchedRecoveryDecls[len(batchedRecoveryDecls)-1] = append(batchedRecoveryDecls[len(batchedRecoveryDecls)-1], miner.RecoveryDeclaration{
			Deadline:  dlIdx,
			Partition: uint64(partIdx),
			Sectors:   recovered,
		})

		totalRecoveries++
	}

	if totalRecoveries == 0 {
		if faulty != 0 {
			log.Warnw("No recoveries to declare", "deadline", dlIdx, "faulty", faulty)
		}

		return nil, nil, nil
	}

	var msgs []*types.SignedMessage
	for _, recovery := range batchedRecoveryDecls {
		params := &miner.DeclareFaultsRecoveredParams{
			Recoveries: recovery,
		}

		enc, aerr := actors.SerializeParams(params)
		if aerr != nil {
			return nil, nil, xerrors.Errorf("could not serialize declare recoveries parameters: %w", aerr)
		}

		msg := &types.Message{
			To:     s.actor,
			Method: builtin.MethodsMiner.DeclareFaultsRecovered,
			Params: enc,
			Value:  types.NewInt(0),
		}
		spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)}
		if err := s.prepareMessage(ctx, msg, spec); err != nil {
			return nil, nil, err
		}

		sm, err := s.api.MpoolPushMessage(ctx, msg, &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)})
		if err != nil {
			return nil, nil, xerrors.Errorf("pushing message to mpool: %w", err)
		}

		log.Warnw("declare faults recovered Message CID", "cid", sm.Cid())
		msgs = append(msgs, sm)
	}

	for _, msg := range msgs {
		rec, err := s.api.StateWaitMsg(context.TODO(), msg.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
		if err != nil {
			return batchedRecoveryDecls, msgs, xerrors.Errorf("declare faults recovered wait error: %w", err)
		}

		if rec.Receipt.ExitCode != 0 {
			return batchedRecoveryDecls, msgs, xerrors.Errorf("declare faults recovered wait non-0 exit code: %d", rec.Receipt.ExitCode)
		}
	}

	return batchedRecoveryDecls, msgs, nil
}

// declareFaults identifies the sectors on the specified proving deadline that
// are faulty, and reports the faults on chain via the `DeclareFaults` message
// to our miner actor.
//
// NOTE: THIS CODE ISN'T INVOKED AFTER THE IGNITION UPGRADE
//
// This is always invoked ahead of time, before the deadline for the evaluated
// sectors arrives. That way, faults are declared before a penalty is accrued.
//
// If a declaration is made, it awaits for build.MessageConfidence confirmations
// on chain before returning.
//
// TODO: the waiting should happen in the background. Right now this
//  is blocking/delaying the actual generation and submission of WindowPoSts in
//  this deadline!
func (s *WindowPoStScheduler) declareFaults(ctx context.Context, dlIdx uint64, partitions []api.Partition, tsk types.TipSetKey) ([]miner.FaultDeclaration, *types.SignedMessage, error) {
	ctx, span := trace.StartSpan(ctx, "storage.declareFaults")
	defer span.End()

	bad := uint64(0)
	params := &miner.DeclareFaultsParams{
		Faults: []miner.FaultDeclaration{},
	}

	for partIdx, partition := range partitions {
		nonFaulty, err := bitfield.SubtractBitField(partition.LiveSectors, partition.FaultySectors)
		if err != nil {
			return nil, nil, xerrors.Errorf("determining non faulty sectors: %w", err)
		}

		good, err := s.checkSectors(ctx, nonFaulty, tsk)
		if err != nil {
			return nil, nil, xerrors.Errorf("checking sectors: %w", err)
		}

		newFaulty, err := bitfield.SubtractBitField(nonFaulty, good)
		if err != nil {
			return nil, nil, xerrors.Errorf("calculating faulty sector set: %w", err)
		}

		c, err := newFaulty.Count()
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
			Sectors:   newFaulty,
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
		Method: builtin.MethodsMiner.DeclareFaults,
		Params: enc,
		Value:  types.NewInt(0), // TODO: Is there a fee?
	}
	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)}
	if err := s.prepareMessage(ctx, msg, spec); err != nil {
		return faults, nil, err
	}

	sm, err := s.api.MpoolPushMessage(ctx, msg, spec)
	if err != nil {
		return faults, sm, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Warnw("declare faults Message CID", "cid", sm.Cid())

	rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		return faults, sm, xerrors.Errorf("declare faults wait error: %w", err)
	}

	if rec.Receipt.ExitCode != 0 {
		return faults, sm, xerrors.Errorf("declare faults wait non-0 exit code: %d", rec.Receipt.ExitCode)
	}

	return faults, sm, nil
}

func (s *WindowPoStScheduler) asyncFaultRecover(di dline.Info, ts *types.TipSet) {
	go func() {
		// check faults / recoveries for the *next* deadline. It's already too
		// late to declare them for this deadline
		declDeadline := (di.Index + 2) % di.WPoStPeriodDeadlines

		partitions, err := s.api.StateMinerPartitions(context.TODO(), s.actor, declDeadline, ts.Key())
		if err != nil {
			log.Errorf("getting partitions: %v", err)
			return
		}

		var (
			sigmsgs    []*types.SignedMessage
			recoveries [][]miner.RecoveryDeclaration

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

		if recoveries, sigmsgs, err = s.declareRecoveries(context.TODO(), declDeadline, partitions, ts.Key()); err != nil {
			// TODO: This is potentially quite bad, but not even trying to post when this fails is objectively worse
			log.Errorf("checking sector recoveries: %v", err)
		}

		// should always be true, skip journaling if not for some reason
		if len(recoveries) == len(sigmsgs) {
			for i, recovery := range recoveries {
				// clone for function literal
				recovery := recovery
				msgCID := optionalCid(sigmsgs[i])
				s.journal.RecordEvent(s.evtTypes[evtTypeWdPoStRecoveries], func() interface{} {
					j := WdPoStRecoveriesProcessedEvt{
						evtCommon:    s.getEvtCommon(err),
						Declarations: recovery,
						MessageCID:   msgCID,
					}
					j.Error = err
					return j
				})
			}
		}
	}()
}
