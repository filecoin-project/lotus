package wdpost

import (
	"bytes"
	"context"
	"time"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-state-types/proof"
	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// recordPoStFailure records a failure in the journal.
//
//nolint:unused
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

	posts, err := s.runPoStCycle(ctx, false, *deadline, ts)
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
		sm, err := s.submitPoStMessage(ctx, post)
		if err != nil {
			log.Errorf("submit window post failed: %+v", err)
			submitErr = err
		} else {
			s.recordProofsEvent(post.Partitions, sm.Cid())
		}
	}

	return submitErr
}

func (s *WindowPoStScheduler) checkSectors(ctx context.Context, check bitfield.BitField, tsk types.TipSetKey) (bitfield.BitField, error) {
	mid, err := address.IDFromAddress(s.actor)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("failed to convert to ID addr: %w", err)
	}

	sectorInfos, err := s.api.StateMinerSectors(ctx, s.actor, &check, tsk)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("failed to get sector infos: %w", err)
	}

	type checkSector struct {
		sealed cid.Cid
		update bool
	}

	sectors := make(map[abi.SectorNumber]checkSector)
	var tocheck []storiface.SectorRef
	for _, info := range sectorInfos {
		sectors[info.SectorNumber] = checkSector{
			sealed: info.SealedCID,
			update: info.SectorKeyCID != nil,
		}
		tocheck = append(tocheck, storiface.SectorRef{
			ProofType: info.SealProof,
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: info.SectorNumber,
			},
		})
	}

	nv, err := s.api.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("failed to get network version: %w", err)
	}

	pp := s.proofType
	// TODO: Drop after nv19 comes and goes
	if nv >= network.Version19 {
		pp, err = pp.ToV1_1PostProof()
		if err != nil {
			return bitfield.BitField{}, xerrors.Errorf("failed to convert to v1_1 post proof: %w", err)
		}
	}

	bad, err := s.faultTracker.CheckProvable(ctx, pp, tocheck, func(ctx context.Context, id abi.SectorID) (cid.Cid, bool, error) {
		s, ok := sectors[id.Number]
		if !ok {
			return cid.Undef, false, xerrors.Errorf("sealed CID not found")
		}
		return s.sealed, s.update, nil
	})
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

// runPoStCycle runs a full cycle of the PoSt process:
//
//  1. performs recovery declarations for the next deadline.
//  2. performs fault declarations for the next deadline.
//  3. computes and submits proofs, batching partitions and making sure they
//     don't exceed message capacity.
//
// When `manual` is set, no messages (fault/recover) will be automatically sent
func (s *WindowPoStScheduler) runPoStCycle(ctx context.Context, manual bool, di dline.Info, ts *types.TipSet) ([]miner.SubmitWindowedPoStParams, error) {
	ctx, span := trace.StartSpan(ctx, "storage.runPoStCycle")
	defer span.End()

	start := time.Now()

	log := log.WithOptions(zap.Fields(zap.Time("cycle", start)))
	log.Infow("starting PoSt cycle", "manual", manual, "ts", ts, "deadline", di.Index)
	defer func() {
		log.Infow("post cycle done", "took", time.Since(start))
	}()

	if !manual {
		// TODO: extract from runPoStCycle, run on fault cutoff boundaries
		s.asyncFaultRecover(di, ts)
	}

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
	partitionBatches, err := s.BatchPartitions(partitions, nv)
	if err != nil {
		log.Errorf("batch partitions failed: %+v", err)
		return nil, err
	}

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("recover: %s", r)
		}
	}()

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
			var xsinfos []proof7.ExtendedSectorInfo
			for partIdx, partition := range batch {
				// TODO: Can do this in parallel
				toProve, err := bitfield.SubtractBitField(partition.LiveSectors, partition.FaultySectors)
				if err != nil {
					return nil, xerrors.Errorf("removing faults from set of sectors to prove: %w", err)
				}
				if manual {
					// this is a check run, we want to prove faulty sectors, even
					// if they are not declared as recovering.
					toProve = partition.LiveSectors
				}
				toProve, err = bitfield.MergeBitFields(toProve, partition.RecoveringSectors)
				if err != nil {
					return nil, xerrors.Errorf("adding recoveries to set of sectors to prove: %w", err)
				}

				good, err := toProve.Copy()
				if err != nil {
					return nil, xerrors.Errorf("copy toProve: %w", err)
				}
				if !s.disablePreChecks {
					good, err = s.checkSectors(ctx, toProve, ts.Key())
					if err != nil {
						return nil, xerrors.Errorf("checking sectors to skip: %w", err)
					}
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

				xsinfos = append(xsinfos, ssi...)
				partitions = append(partitions, miner.PoStPartition{
					Index:   uint64(batchPartitionStartIdx + partIdx),
					Skipped: skipped,
				})
			}

			if len(xsinfos) == 0 {
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

			ppt, err := xsinfos[0].SealProof.RegisteredWindowPoStProofByNetworkVersion(nv)
			if err != nil {
				return nil, xerrors.Errorf("failed to get window post type: %w", err)
			}

			postOut, ps, err := s.prover.GenerateWindowPoSt(ctx, abi.ActorID(mid), ppt, xsinfos, append(abi.PoStRandomness{}, rand...))
			elapsed := time.Since(tsStart)
			log.Infow("computing window post", "batch", batchIdx, "elapsed", elapsed, "skip", len(ps), "err", err)
			if err != nil {
				log.Errorf("error generating window post: %s", err)
			}
			if err == nil {

				// If we proved nothing, something is very wrong.
				if len(postOut) == 0 {
					log.Errorf("len(postOut) == 0")
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
				sinfos := make([]proof7.SectorInfo, len(xsinfos))
				for i, xsi := range xsinfos {
					sinfos[i] = proof7.SectorInfo{
						SealProof:    xsi.SealProof,
						SectorNumber: xsi.SectorNumber,
						SealedCID:    xsi.SealedCID,
					}
				}
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
			log.Debugf("Proof generation failed, retry")
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

// BatchPartitions splits partitions into batches of partitions, so as not to exceed the number of
// sectors allowed in a single message.
//
// Note: Partition order within batches must match original partition order in order
// for code following the user code to work
func (s *WindowPoStScheduler) BatchPartitions(partitions []api.Partition, nv network.Version) ([][]api.Partition, error) {
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
	if partitionsPerMsg > policy.DeclarationsMax {
		partitionsPerMsg = policy.DeclarationsMax
	}

	// respect user config if set
	if s.maxPartitionsPerPostMessage > 0 {
		if partitionsPerMsg > s.maxPartitionsPerPostMessage {
			partitionsPerMsg = s.maxPartitionsPerPostMessage
		}
	}

	batches := [][]api.Partition{}

	currBatch := []api.Partition{}
	for _, partition := range partitions {
		recSectors, err := partition.RecoveringSectors.Count()
		if err != nil {
			return nil, err
		}

		// Only add single partition to a batch if it contains recovery sectors
		// and has the below user config set
		if s.singleRecoveringPartitionPerPostMessage && recSectors > 0 {
			if len(currBatch) > 0 {
				batches = append(batches, currBatch)
				currBatch = []api.Partition{}
			}
			batches = append(batches, []api.Partition{partition})
		} else {
			if len(currBatch) >= partitionsPerMsg {
				batches = append(batches, currBatch)
				currBatch = []api.Partition{}
			}
			currBatch = append(currBatch, partition)
		}
	}
	if len(currBatch) > 0 {
		batches = append(batches, currBatch)
	}

	return batches, nil
}

func (s *WindowPoStScheduler) sectorsForProof(ctx context.Context, goodSectors, allSectors bitfield.BitField, ts *types.TipSet) ([]proof7.ExtendedSectorInfo, error) {
	sset, err := s.api.StateMinerSectors(ctx, s.actor, &goodSectors, ts.Key())
	if err != nil {
		return nil, err
	}

	if len(sset) == 0 {
		return nil, nil
	}

	substitute := proof7.ExtendedSectorInfo{
		SectorNumber: sset[0].SectorNumber,
		SealedCID:    sset[0].SealedCID,
		SealProof:    sset[0].SealProof,
		SectorKey:    sset[0].SectorKeyCID,
	}

	sectorByID := make(map[uint64]proof7.ExtendedSectorInfo, len(sset))
	for _, sector := range sset {
		sectorByID[uint64(sector.SectorNumber)] = proof7.ExtendedSectorInfo{
			SectorNumber: sector.SectorNumber,
			SealedCID:    sector.SealedCID,
			SealProof:    sector.SealProof,
			SectorKey:    sector.SectorKeyCID,
		}
	}

	proofSectors := make([]proof7.ExtendedSectorInfo, 0, len(sset))
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
		Method: builtin.MethodsMiner.SubmitWindowedPoSt,
		Params: enc,
		Value:  types.NewInt(0),
	}
	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(s.feeCfg.MaxWindowPoStGasFee), MaximizeFeeCap: s.feeCfg.MaximizeWindowPoStFeeCap}
	if err := s.prepareMessage(ctx, msg, spec); err != nil {
		return nil, err
	}

	sm, err := s.api.MpoolPushMessage(ctx, msg, spec)
	if err != nil {
		return nil, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	log.Infof("Submitted window post: %s (deadline %d)", sm.Cid(), proof.Deadline)

	go func() {
		rec, err := s.api.StateWaitMsg(context.TODO(), sm.Cid(), buildconstants.MessageConfidence, api.LookbackNoLimit, true)
		if err != nil {
			log.Error(err)
			return
		}

		if rec.Receipt.ExitCode == 0 {
			log.Infow("Window post submission successful", "cid", sm.Cid(), "deadline", proof.Deadline, "epoch", rec.Height, "ts", rec.TipSet.Cids())
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

func (s *WindowPoStScheduler) ComputePoSt(ctx context.Context, dlIdx uint64, ts *types.TipSet) ([]miner.SubmitWindowedPoStParams, error) {
	dl, err := s.api.StateMinerProvingDeadline(ctx, s.actor, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting deadline: %w", err)
	}
	curIdx := dl.Index
	dl.Index = dlIdx
	dlDiff := dl.Index - curIdx
	if dl.Index > curIdx {
		dlDiff -= dl.WPoStPeriodDeadlines
		dl.PeriodStart -= dl.WPoStProvingPeriod
	}

	epochDiff := (dl.WPoStProvingPeriod / abi.ChainEpoch(dl.WPoStPeriodDeadlines)) * abi.ChainEpoch(dlDiff)

	// runPoStCycle only needs dl.Index and dl.Challenge
	dl.Challenge += epochDiff

	return s.runPoStCycle(ctx, true, *dl, ts)
}

func (s *WindowPoStScheduler) ManualFaultRecovery(ctx context.Context, maddr address.Address, sectors []abi.SectorNumber) ([]cid.Cid, error) {
	return s.declareManualRecoveries(ctx, maddr, sectors, types.TipSetKey{})
}
