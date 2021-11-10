package storage

import (
	"bytes"
	"context"
	"time"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/ipfs/go-cid"

	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	proof2 "github.com/filecoin-project/specs-actors/v2/actors/runtime/proof"
	"github.com/filecoin-project/specs-actors/v3/actors/runtime/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
)

// recordPoStFailure records a failure in the journal.
func (s *WindowPoStScheduler) recordPoStFailure(err error, ts *types.TipSet, deadline *dline.Info) {
	s.journal.RecordEvent(s.evtTypes[evtTypeWdPoStScheduler], func() interface{} {
		c := evtCommon{Error: err}
		if ts != nil {
			c.Deadline = deadline
			c.Height = ts.Height()
			c.TipSet = ts.Cids()
		}
		return WdPoStSchedulerEvt{
			evtCommon: c,
			State:     SchedulerStateFaulted,
		}
	})

	log.Errorf("Got err %+v - TODO handle errors", err)
	/*s.failLk.Lock()
	if eps > s.failed {
		s.failed = eps
	}
	s.failLk.Unlock()*/
}

// recordProofsEvent records a successful proofs_processed event in the
// journal, even if it was a noop (no partitions).
func (s *WindowPoStScheduler) recordProofsEvent(partitions []miner.PoStPartition, mcid cid.Cid) {
	s.journal.RecordEvent(s.evtTypes[evtTypeWdPoStProofs], func() interface{} {
		return &WdPoStProofsProcessedEvt{
			evtCommon:  s.getEvtCommon(nil),
			Partitions: partitions,
			MessageCID: mcid,
		}
	})
}

// startGeneratePoST kicks off the process of generating a PoST
func (s *WindowPoStScheduler) startGeneratePoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	completeGeneratePoST CompleteGeneratePoSTCb,
) context.CancelFunc {
	ctx, abort := context.WithCancel(ctx)
	go func() {
		defer abort()

		s.journal.RecordEvent(s.evtTypes[evtTypeWdPoStScheduler], func() interface{} {
			return WdPoStSchedulerEvt{
				evtCommon: s.getEvtCommon(nil),
				State:     SchedulerStateStarted,
			}
		})

		posts, err := s.runGeneratePoST(ctx, ts, deadline)
		completeGeneratePoST(posts, err)
	}()

	return abort
}

// runGeneratePoST generates the PoST
func (s *WindowPoStScheduler) runGeneratePoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
) ([]miner.SubmitWindowedPoStParams, error) {
	ctx, span := trace.StartSpan(ctx, "WindowPoStScheduler.generatePoST")
	defer span.End()

	posts, err := s.runPoStCycle(ctx, *deadline, ts)
	if err != nil {
		log.Errorf("runPoStCycle failed: %+v", err)
		return nil, err
	}

	if len(posts) == 0 {
		s.recordProofsEvent(nil, cid.Undef)
	}

	return posts, nil
}

// startSubmitPoST kicks of the process of submitting PoST
func (s *WindowPoStScheduler) startSubmitPoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	posts []miner.SubmitWindowedPoStParams,
	completeSubmitPoST CompleteSubmitPoSTCb,
) context.CancelFunc {

	ctx, abort := context.WithCancel(ctx)
	go func() {
		defer abort()

		err := s.runSubmitPoST(ctx, ts, deadline, posts)
		if err == nil {
			s.journal.RecordEvent(s.evtTypes[evtTypeWdPoStScheduler], func() interface{} {
				return WdPoStSchedulerEvt{
					evtCommon: s.getEvtCommon(nil),
					State:     SchedulerStateSucceeded,
				}
			})
		}
		completeSubmitPoST(err)
	}()

	return abort
}

// runSubmitPoST submits PoST
func (s *WindowPoStScheduler) runSubmitPoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	posts []miner.SubmitWindowedPoStParams,
) error {
	if len(posts) == 0 {
		return nil
	}

	ctx, span := trace.StartSpan(ctx, "WindowPoStScheduler.submitPoST")
	defer span.End()

	// Get randomness from tickets
	// use the challenge epoch if we've upgraded to network version 4
	// (actors version 2). We want to go back as far as possible to be safe.
	commEpoch := deadline.Open
	if ver, err := s.api.StateNetworkVersion(ctx, types.EmptyTSK); err != nil {
		log.Errorw("failed to get network version to determine PoSt epoch randomness lookback", "error", err)
	} else if ver >= network.Version4 {
		commEpoch = deadline.Challenge
	}

	commRand, err := s.api.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, commEpoch, nil, ts.Key())
	if err != nil {
		err = xerrors.Errorf("failed to get chain randomness from tickets for windowPost (ts=%d; deadline=%d): %w", ts.Height(), commEpoch, err)
		log.Errorf("submitPoStMessage failed: %+v", err)

		return err
	}

	var submitErr error
	for i := range posts {
		// Add randomness to PoST
		post := &posts[i]
		post.ChainCommitEpoch = commEpoch
		post.ChainCommitRand = commRand

		// Submit PoST
		sm, submitErr := s.submitPoStMessage(ctx, post)
		if submitErr != nil {
			log.Errorf("submit window post failed: %+v", submitErr)
		} else {
			s.recordProofsEvent(post.Partitions, sm.Cid())
		}
	}

	return submitErr
}

func (s *WindowPoStScheduler) checkSectors(ctx context.Context, check bitfield.BitField, tsk types.TipSetKey) (bitfield.BitField, error) {
	mid, err := address.IDFromAddress(s.actor)
	if err != nil {
		return bitfield.BitField{}, err
	}

	sectorInfos, err := s.api.StateMinerSectors(ctx, s.actor, &check, tsk)
	if err != nil {
		return bitfield.BitField{}, err
	}

	sectors := make(map[abi.SectorNumber]struct{})
	var tocheck []storage.SectorRef
	for _, info := range sectorInfos {
		sectors[info.SectorNumber] = struct{}{}
		tocheck = append(tocheck, storage.SectorRef{
			ProofType: info.SealProof,
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: info.SectorNumber,
			},
		})
	}

	bad, err := s.faultTracker.CheckProvable(ctx, s.proofType, tocheck, nil)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("checking provable sectors: %w", err)
	}
	for id := range bad {
		delete(sectors, id.Number)
	}

	log.Warnw("Checked sectors", "checked", len(tocheck), "good", len(sectors))

	sbf := bitfield.New()
	for s := range sectors {
		sbf.Set(uint64(s))
	}

	return sbf, nil
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
// If a declaration is made, it awaits for build.MessageConfidence confirmations
// on chain before returning.
//
// TODO: the waiting should happen in the background. Right now this
//  is blocking/delaying the actual generation and submission of WindowPoSts in
//  this deadline!
func (s *WindowPoStScheduler) declareRecoveries(ctx context.Context, dlIdx uint64, partitions []api.Partition, tsk types.TipSetKey) ([]miner.RecoveryDeclaration, *types.SignedMessage, error) {
	ctx, span := trace.StartSpan(ctx, "storage.declareRecoveries")
	defer span.End()

	faulty := uint64(0)
	params := &miner.DeclareFaultsRecoveredParams{
		Recoveries: []miner.RecoveryDeclaration{},
	}

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
		Method: miner.Methods.DeclareFaultsRecovered,
		Params: enc,
		Value:  types.NewInt(0),
	}
	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)}
	if err := s.prepareMessage(ctx, msg, spec); err != nil {
		return recoveries, nil, err
	}

	sm, err := s.api.MpoolPushMessage(ctx, msg, &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)})
	if err != nil {
		return recoveries, sm, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Warnw("declare faults recovered Message CID", "cid", sm.Cid())

	rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		return recoveries, sm, xerrors.Errorf("declare faults recovered wait error: %w", err)
	}

	if rec.Receipt.ExitCode != 0 {
		return recoveries, sm, xerrors.Errorf("declare faults recovered wait non-0 exit code: %d", rec.Receipt.ExitCode)
	}

	return recoveries, sm, nil
}

// declareFaults identifies the sectors on the specified proving deadline that
// are faulty, and reports the faults on chain via the `DeclareFaults` message
// to our miner actor.
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
		Method: miner.Methods.DeclareFaults,
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

// runPoStCycle runs a full cycle of the PoSt process:
//
//  1. performs recovery declarations for the next deadline.
//  2. performs fault declarations for the next deadline.
//  3. computes and submits proofs, batching partitions and making sure they
//     don't exceed message capacity.
func (s *WindowPoStScheduler) runPoStCycle(ctx context.Context, di dline.Info, ts *types.TipSet) ([]miner.SubmitWindowedPoStParams, error) {
	ctx, span := trace.StartSpan(ctx, "storage.runPoStCycle")
	defer span.End()

	go func() {
		// TODO: extract from runPoStCycle, run on fault cutoff boundaries

		// check faults / recoveries for the *next* deadline. It's already too
		// late to declare them for this deadline
		declDeadline := (di.Index + 2) % di.WPoStPeriodDeadlines

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

		if recoveries, sigmsg, err = s.declareRecoveries(context.TODO(), declDeadline, partitions, ts.Key()); err != nil {
			// TODO: This is potentially quite bad, but not even trying to post when this fails is objectively worse
			log.Errorf("checking sector recoveries: %v", err)
		}

		s.journal.RecordEvent(s.evtTypes[evtTypeWdPoStRecoveries], func() interface{} {
			j := WdPoStRecoveriesProcessedEvt{
				evtCommon:    s.getEvtCommon(err),
				Declarations: recoveries,
				MessageCID:   optionalCid(sigmsg),
			}
			j.Error = err
			return j
		})

		if ts.Height() > build.UpgradeIgnitionHeight {
			return // FORK: declaring faults after ignition upgrade makes no sense
		}

		if faults, sigmsg, err = s.declareFaults(context.TODO(), declDeadline, partitions, ts.Key()); err != nil {
			// TODO: This is also potentially really bad, but we try to post anyways
			log.Errorf("checking sector faults: %v", err)
		}

		s.journal.RecordEvent(s.evtTypes[evtTypeWdPoStFaults], func() interface{} {
			return WdPoStFaultsProcessedEvt{
				evtCommon:    s.getEvtCommon(err),
				Declarations: faults,
				MessageCID:   optionalCid(sigmsg),
			}
		})
	}()

	buf := new(bytes.Buffer)
	if err := s.actor.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal address to cbor: %w", err)
	}

	headTs, err := s.api.ChainHead(ctx)
	if err != nil {
		return nil, xerrors.Errorf("getting current head: %w", err)
	}

	rand, err := s.api.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, buf.Bytes(), headTs.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get chain randomness from beacon for window post (ts=%d; deadline=%d): %w", ts.Height(), di, err)
	}

	// Get the partitions for the given deadline
	partitions, err := s.api.StateMinerPartitions(ctx, s.actor, di.Index, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting partitions: %w", err)
	}

	nv, err := s.api.StateNetworkVersion(ctx, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting network version: %w", err)
	}

	// Split partitions into batches, so as not to exceed the number of sectors
	// allowed in a single message
	partitionBatches, err := s.batchPartitions(partitions, nv)
	if err != nil {
		return nil, err
	}

	// Generate proofs in batches
	posts := make([]miner.SubmitWindowedPoStParams, 0, len(partitionBatches))
	for batchIdx, batch := range partitionBatches {
		batchPartitionStartIdx := 0
		for _, batch := range partitionBatches[:batchIdx] {
			batchPartitionStartIdx += len(batch)
		}

		params := miner.SubmitWindowedPoStParams{
			Deadline:   di.Index,
			Partitions: make([]miner.PoStPartition, 0, len(batch)),
			Proofs:     nil,
		}

		postSkipped := bitfield.New()
		somethingToProve := false

		// Retry until we run out of sectors to prove.
		for retries := 0; ; retries++ {
			skipCount := uint64(0)
			var partitions []miner.PoStPartition
			var sinfos []proof2.SectorInfo
			for partIdx, partition := range batch {
				// TODO: Can do this in parallel
				toProve, err := bitfield.SubtractBitField(partition.LiveSectors, partition.FaultySectors)
				if err != nil {
					return nil, xerrors.Errorf("removing faults from set of sectors to prove: %w", err)
				}
				toProve, err = bitfield.MergeBitFields(toProve, partition.RecoveringSectors)
				if err != nil {
					return nil, xerrors.Errorf("adding recoveries to set of sectors to prove: %w", err)
				}

				good, err := s.checkSectors(ctx, toProve, ts.Key())
				if err != nil {
					return nil, xerrors.Errorf("checking sectors to skip: %w", err)
				}

				good, err = bitfield.SubtractBitField(good, postSkipped)
				if err != nil {
					return nil, xerrors.Errorf("toProve - postSkipped: %w", err)
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

				ssi, err := s.sectorsForProof(ctx, good, partition.AllSectors, ts)
				if err != nil {
					return nil, xerrors.Errorf("getting sorted sector info: %w", err)
				}

				if len(ssi) == 0 {
					continue
				}

				sinfos = append(sinfos, ssi...)
				partitions = append(partitions, miner.PoStPartition{
					Index:   uint64(batchPartitionStartIdx + partIdx),
					Skipped: skipped,
				})
			}

			if len(sinfos) == 0 {
				// nothing to prove for this batch
				break
			}

			// Generate proof
			log.Infow("running window post",
				"chain-random", rand,
				"deadline", di,
				"height", ts.Height(),
				"skipped", skipCount)

			tsStart := build.Clock.Now()

			mid, err := address.IDFromAddress(s.actor)
			if err != nil {
				return nil, err
			}

			postOut, ps, err := s.prover.GenerateWindowPoSt(ctx, abi.ActorID(mid), sinfos, append(abi.PoStRandomness{}, rand...))
			elapsed := time.Since(tsStart)

			log.Infow("computing window post", "batch", batchIdx, "elapsed", elapsed)

			if err == nil {
				// If we proved nothing, something is very wrong.
				if len(postOut) == 0 {
					return nil, xerrors.Errorf("received no proofs back from generate window post")
				}

				headTs, err := s.api.ChainHead(ctx)
				if err != nil {
					return nil, xerrors.Errorf("getting current head: %w", err)
				}

				checkRand, err := s.api.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, buf.Bytes(), headTs.Key())
				if err != nil {
					return nil, xerrors.Errorf("failed to get chain randomness from beacon for window post (ts=%d; deadline=%d): %w", ts.Height(), di, err)
				}

				if !bytes.Equal(checkRand, rand) {
					log.Warnw("windowpost randomness changed", "old", rand, "new", checkRand, "ts-height", ts.Height(), "challenge-height", di.Challenge, "tsk", ts.Key())
					rand = checkRand
					continue
				}

				// If we generated an incorrect proof, try again.
				if correct, err := s.verifier.VerifyWindowPoSt(ctx, proof.WindowPoStVerifyInfo{
					Randomness:        abi.PoStRandomness(checkRand),
					Proofs:            postOut,
					ChallengedSectors: sinfos,
					Prover:            abi.ActorID(mid),
				}); err != nil {
					log.Errorw("window post verification failed", "post", postOut, "error", err)
					time.Sleep(5 * time.Second)
					continue
				} else if !correct {
					log.Errorw("generated incorrect window post proof", "post", postOut, "error", err)
					continue
				}

				// Proof generation successful, stop retrying
				somethingToProve = true
				params.Partitions = partitions
				params.Proofs = postOut
				break
			}

			// Proof generation failed, so retry

			if len(ps) == 0 {
				// If we didn't skip any new sectors, we failed
				// for some other reason and we need to abort.
				return nil, xerrors.Errorf("running window post failed: %w", err)
			}
			// TODO: maybe mark these as faulty somewhere?

			log.Warnw("generate window post skipped sectors", "sectors", ps, "error", err, "try", retries)

			// Explicitly make sure we haven't aborted this PoSt
			// (GenerateWindowPoSt may or may not check this).
			// Otherwise, we could try to continue proving a
			// deadline after the deadline has ended.
			if ctx.Err() != nil {
				log.Warnw("aborting PoSt due to context cancellation", "error", ctx.Err(), "deadline", di.Index)
				return nil, ctx.Err()
			}

			for _, sector := range ps {
				postSkipped.Set(uint64(sector.Number))
			}
		}

		// Nothing to prove for this batch, try the next batch
		if !somethingToProve {
			continue
		}

		posts = append(posts, params)
	}

	return posts, nil
}

func (s *WindowPoStScheduler) batchPartitions(partitions []api.Partition, nv network.Version) ([][]api.Partition, error) {
	// We don't want to exceed the number of sectors allowed in a message.
	// So given the number of sectors in a partition, work out the number of
	// partitions that can be in a message without exceeding sectors per
	// message:
	// floor(number of sectors allowed in a message / sectors per partition)
	// eg:
	// max sectors per message  7:  ooooooo
	// sectors per partition    3:  ooo
	// partitions per message   2:  oooOOO
	//                              <1><2> (3rd doesn't fit)
	partitionsPerMsg, err := policy.GetMaxPoStPartitions(nv, s.proofType)
	if err != nil {
		return nil, xerrors.Errorf("getting sectors per partition: %w", err)
	}

	// Also respect the AddressedPartitionsMax (which is the same as DeclarationsMax (which is all really just MaxPartitionsPerDeadline))
	declMax, err := policy.GetDeclarationsMax(nv)
	if err != nil {
		return nil, xerrors.Errorf("getting max declarations: %w", err)
	}
	if partitionsPerMsg > declMax {
		partitionsPerMsg = declMax
	}

	// The number of messages will be:
	// ceiling(number of partitions / partitions per message)
	batchCount := len(partitions) / partitionsPerMsg
	if len(partitions)%partitionsPerMsg != 0 {
		batchCount++
	}

	// Split the partitions into batches
	batches := make([][]api.Partition, 0, batchCount)
	for i := 0; i < len(partitions); i += partitionsPerMsg {
		end := i + partitionsPerMsg
		if end > len(partitions) {
			end = len(partitions)
		}
		batches = append(batches, partitions[i:end])
	}

	return batches, nil
}

func (s *WindowPoStScheduler) sectorsForProof(ctx context.Context, goodSectors, allSectors bitfield.BitField, ts *types.TipSet) ([]proof2.SectorInfo, error) {
	sset, err := s.api.StateMinerSectors(ctx, s.actor, &goodSectors, ts.Key())
	if err != nil {
		return nil, err
	}

	if len(sset) == 0 {
		return nil, nil
	}

	substitute := proof2.SectorInfo{
		SectorNumber: sset[0].SectorNumber,
		SealedCID:    sset[0].SealedCID,
		SealProof:    sset[0].SealProof,
	}

	sectorByID := make(map[uint64]proof2.SectorInfo, len(sset))
	for _, sector := range sset {
		sectorByID[uint64(sector.SectorNumber)] = proof2.SectorInfo{
			SectorNumber: sector.SectorNumber,
			SealedCID:    sector.SealedCID,
			SealProof:    sector.SealProof,
		}
	}

	proofSectors := make([]proof2.SectorInfo, 0, len(sset))
	if err := allSectors.ForEach(func(sectorNo uint64) error {
		if info, found := sectorByID[sectorNo]; found {
			proofSectors = append(proofSectors, info)
		} else {
			proofSectors = append(proofSectors, substitute)
		}
		return nil
	}); err != nil {
		return nil, xerrors.Errorf("iterating partition sector bitmap: %w", err)
	}

	return proofSectors, nil
}

// submitPoStMessage builds a SubmitWindowedPoSt message and submits it to
// the mpool. It doesn't synchronously block on confirmations, but it does
// monitor in the background simply for the purposes of logging.
func (s *WindowPoStScheduler) submitPoStMessage(ctx context.Context, proof *miner.SubmitWindowedPoStParams) (*types.SignedMessage, error) {
	ctx, span := trace.StartSpan(ctx, "storage.commitPost")
	defer span.End()

	var sm *types.SignedMessage

	enc, aerr := actors.SerializeParams(proof)
	if aerr != nil {
		return nil, xerrors.Errorf("could not serialize submit window post parameters: %w", aerr)
	}

	msg := &types.Message{
		To:     s.actor,
		Method: miner.Methods.SubmitWindowedPoSt,
		Params: enc,
		Value:  types.NewInt(0),
	}
	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee)}
	if err := s.prepareMessage(ctx, msg, spec); err != nil {
		return nil, err
	}

	sm, err := s.api.MpoolPushMessage(ctx, msg, spec)
	if err != nil {
		return nil, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Infof("Submitted window post: %s", sm.Cid())

	go func() {
		rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
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

// prepareMessage prepares a message before sending it, setting:
//
// * the sender (from the AddressSelector, falling back to the worker address if none set)
// * the right gas parameters
func (s *WindowPoStScheduler) prepareMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) error {
	mi, err := s.api.StateMinerInfo(ctx, s.actor, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("error getting miner info: %w", err)
	}
	// set the worker as a fallback
	msg.From = mi.Worker

	// (optimal) initial estimation with some overestimation that guarantees
	// block inclusion within the next 20 tipsets.
	gm, err := s.api.GasEstimateMessageGas(ctx, msg, spec, types.EmptyTSK)
	if err != nil {
		log.Errorw("estimating gas", "error", err)
		return nil
	}
	*msg = *gm

	// calculate a more frugal estimation; premium is estimated to guarantee
	// inclusion within 5 tipsets, and fee cap is estimated for inclusion
	// within 4 tipsets.
	minGasFeeMsg := *msg

	minGasFeeMsg.GasPremium, err = s.api.GasEstimateGasPremium(ctx, 5, msg.From, msg.GasLimit, types.EmptyTSK)
	if err != nil {
		log.Errorf("failed to estimate minimum gas premium: %+v", err)
		minGasFeeMsg.GasPremium = msg.GasPremium
	}

	minGasFeeMsg.GasFeeCap, err = s.api.GasEstimateFeeCap(ctx, &minGasFeeMsg, 4, types.EmptyTSK)
	if err != nil {
		log.Errorf("failed to estimate minimum gas fee cap: %+v", err)
		minGasFeeMsg.GasFeeCap = msg.GasFeeCap
	}

	// goodFunds = funds needed for optimal inclusion probability.
	// minFunds  = funds needed for more speculative inclusion probability.
	goodFunds := big.Add(msg.RequiredFunds(), msg.Value)
	minFunds := big.Min(big.Add(minGasFeeMsg.RequiredFunds(), minGasFeeMsg.Value), goodFunds)

	pa, avail, err := s.addrSel.AddressFor(ctx, s.api, mi, api.PoStAddr, goodFunds, minFunds)
	if err != nil {
		log.Errorw("error selecting address for window post", "error", err)
		return nil
	}

	msg.From = pa
	bestReq := big.Add(msg.RequiredFunds(), msg.Value)
	if avail.LessThan(bestReq) {
		mff := func() (abi.TokenAmount, error) {
			return msg.RequiredFunds(), nil
		}

		messagepool.CapGasFee(mff, msg, &api.MessageSendSpec{MaxFee: big.Min(big.Sub(avail, msg.Value), msg.RequiredFunds())})
	}
	return nil
}
