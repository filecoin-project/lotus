package wdpost

import (
	"context"
	"math"
	"os"
	"strconv"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

var RecoveringSectorLimit uint64

func init() {
	if rcl := os.Getenv("LOTUS_RECOVERING_SECTOR_LIMIT"); rcl != "" {
		var err error
		RecoveringSectorLimit, err = strconv.ParseUint(rcl, 10, 64)
		if err != nil {
			log.Errorw("parsing LOTUS_RECOVERING_SECTOR_LIMIT", "error", err)
		}
	}
}

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
// If a declaration is made, it awaits for buildconstants.MessageConfidence confirmations
// on chain before returning.
//
// TODO: the waiting should happen in the background. Right now this
//
//	is blocking/delaying the actual generation and submission of WindowPoSts in
//	this deadline!
func (s *WindowPoStScheduler) declareRecoveries(ctx context.Context, dlIdx uint64, partitions []api.Partition, tsk types.TipSetKey) ([][]miner.RecoveryDeclaration, []*types.SignedMessage, error) {
	ctx, span := trace.StartSpan(ctx, "storage.declareRecoveries")
	defer span.End()

	faulty := uint64(0)

	var batchedRecoveryDecls [][]miner.RecoveryDeclaration
	batchedRecoveryDecls = append(batchedRecoveryDecls, []miner.RecoveryDeclaration{})
	totalSectorsToRecover := uint64(0)

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

		// rules to follow if we have indicated that we don't want to recover more than X sectors in a deadline
		if RecoveringSectorLimit > 0 {
			// something weird happened, break because we can't recover any more
			if RecoveringSectorLimit < totalSectorsToRecover {
				log.Warnf("accepted more recoveries (%d) than RecoveringSectorLimit (%d)", totalSectorsToRecover, RecoveringSectorLimit)
				break
			}

			maxNewRecoverable := RecoveringSectorLimit - totalSectorsToRecover

			// we need to trim the recover bitfield
			if recoveredCount > maxNewRecoverable {
				recoverySlice, err := recovered.All(math.MaxUint64)
				if err != nil {
					log.Errorw("failed to slice recovery bitfield, breaking out of recovery loop", err)
					break
				}

				log.Warnf("only adding %d sectors to respect RecoveringSectorLimit %d", maxNewRecoverable, RecoveringSectorLimit)

				recovered = bitfield.NewFromSet(recoverySlice[:maxNewRecoverable])
				recoveredCount = maxNewRecoverable
			}
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

		totalSectorsToRecover += recoveredCount

		if RecoveringSectorLimit > 0 && totalSectorsToRecover >= RecoveringSectorLimit {
			log.Errorf("reached recovering sector limit %d, only marking %d sectors for recovery now",
				RecoveringSectorLimit,
				totalSectorsToRecover)
			break
		}
	}

	if totalSectorsToRecover == 0 {
		if faulty != 0 {
			log.Warnw("No recoveries to declare", "deadline", dlIdx, "faulty", faulty)
		}

		return nil, nil, nil
	}

	log.Infof("attempting recovery declarations for %d sectors", totalSectorsToRecover)
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
		spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee), MaximizeFeeCap: s.feeCfg.MaximizeWindowPoStFeeCap}
		if err := s.prepareMessage(ctx, msg, spec); err != nil {
			return nil, nil, err
		}
		sm, err := s.api.MpoolPushMessage(ctx, msg, spec)
		if err != nil {
			return nil, nil, xerrors.Errorf("pushing message to mpool: %w", err)
		}

		log.Warnw("declare faults recovered Message CID", "cid", sm.Cid())
		msgs = append(msgs, sm)
	}

	for _, msg := range msgs {
		rec, err := s.api.StateWaitMsg(context.TODO(), msg.Cid(), buildconstants.MessageConfidence, api.LookbackNoLimit, true)
		if err != nil {
			return batchedRecoveryDecls, msgs, xerrors.Errorf("declare faults recovered wait error: %w", err)
		}

		if rec.Receipt.ExitCode != 0 {
			return batchedRecoveryDecls, msgs, xerrors.Errorf("declare faults recovered wait non-0 exit code: %d", rec.Receipt.ExitCode)
		}
	}

	return batchedRecoveryDecls, msgs, nil
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

// declareManualRecoveries identifies sectors that were previously marked as faulty
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
// If a declaration is made, it awaits for buildconstants.MessageConfidence confirmations
// on chain before returning.
func (s *WindowPoStScheduler) declareManualRecoveries(ctx context.Context, maddr address.Address, sectors []abi.SectorNumber, tsk types.TipSetKey) ([]cid.Cid, error) {

	var RecoveryDecls []miner.RecoveryDeclaration
	var RecoveryBatches [][]miner.RecoveryDeclaration

	type ptx struct {
		deadline  uint64
		partition uint64
	}

	smap := make(map[ptx][]uint64)

	var mcids []cid.Cid

	for _, sector := range sectors {
		ptxID, err := s.api.StateSectorPartition(ctx, maddr, sector, types.TipSetKey{})
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch partition and deadline details for sector %d: %w", sector, err)
		}
		ptxinfo := ptx{
			deadline:  ptxID.Deadline,
			partition: ptxID.Partition,
		}

		slist := smap[ptxinfo]
		sn := uint64(sector)
		slist = append(slist, sn)
		smap[ptxinfo] = slist
	}

	for i, v := range smap {
		sectorinbit := bitfield.NewFromSet(v)
		RecoveryDecls = append(RecoveryDecls, miner.RecoveryDeclaration{
			Deadline:  i.deadline,
			Partition: i.partition,
			Sectors:   sectorinbit,
		})
	}

	// Batch if maxPartitionsPerRecoveryMessage is set
	if s.maxPartitionsPerRecoveryMessage > 0 {

		// Create batched
		for len(RecoveryDecls) > s.maxPartitionsPerPostMessage {
			Batch := RecoveryDecls[len(RecoveryDecls)-s.maxPartitionsPerRecoveryMessage:]
			RecoveryDecls = RecoveryDecls[:len(RecoveryDecls)-s.maxPartitionsPerPostMessage]
			RecoveryBatches = append(RecoveryBatches, Batch)
		}

		// Add remaining as new batch
		RecoveryBatches = append(RecoveryBatches, RecoveryDecls)
	} else {
		RecoveryBatches = append(RecoveryBatches, RecoveryDecls)
	}

	for _, Batch := range RecoveryBatches {
		msg, err := s.manualRecoveryMsg(ctx, Batch)
		if err != nil {
			return nil, err
		}

		mcids = append(mcids, msg)
	}

	return mcids, nil
}

func (s *WindowPoStScheduler) manualRecoveryMsg(ctx context.Context, Recovery []miner.RecoveryDeclaration) (cid.Cid, error) {
	params := &miner.DeclareFaultsRecoveredParams{
		Recoveries: Recovery,
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return cid.Undef, xerrors.Errorf("could not serialize declare recoveries parameters: %w", aerr)
	}

	msg := &types.Message{
		To:     s.actor,
		Method: builtin.MethodsMiner.DeclareFaultsRecovered,
		Params: enc,
		Value:  types.NewInt(0),
	}
	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)}
	if err := s.prepareMessage(ctx, msg, spec); err != nil {
		return cid.Undef, err
	}
	sm, err := s.api.MpoolPushMessage(ctx, msg, &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)})
	if err != nil {
		return cid.Undef, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	return sm.Cid(), nil
}
