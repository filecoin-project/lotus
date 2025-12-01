package miner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/arc/v2"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	lrand "github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/journal"
)

var log = logging.Logger("miner")

// Journal event types.
const (
	evtTypeBlockMined = iota
)

// waitFunc is expected to pace block mining at the configured network rate.
//
// baseTime is the timestamp of the mining base, i.e. the timestamp
// of the tipset we're planning to construct upon.
//
// Upon each mining loop iteration, the returned callback is called reporting
// whether we mined a block in this round or not.
type waitFunc func(ctx context.Context, baseTime uint64) (func(bool, abi.ChainEpoch, error), abi.ChainEpoch, error)

func randTimeOffset(width time.Duration) time.Duration {
	buf := make([]byte, 8)
	_, _ = rand.Reader.Read(buf)
	val := time.Duration(binary.BigEndian.Uint64(buf) % uint64(width))

	return val - (width / 2)
}

// NewMiner instantiates a miner with a concrete WinningPoStProver and a miner
// address (which can be different from the worker's address).
func NewMiner(api v1api.FullNode, epp gen.WinningPoStProver, addr address.Address, sf *slashfilter.SlashFilter, j journal.Journal) *Miner {
	arc, err := arc.NewARC[abi.ChainEpoch, bool](10000)
	if err != nil {
		panic(err)
	}

	return &Miner{
		api:     api,
		epp:     epp,
		address: addr,
		propagationWaitFunc: func(ctx context.Context, baseTime uint64) (func(bool, abi.ChainEpoch, error), abi.ChainEpoch, error) {
			// wait around for half the block time in case other parents come in
			//
			// if we're mining a block in the past via catch-up/rush mining,
			// such as when recovering from a network halt, this sleep will be
			// for a negative duration, and therefore **will return
			// immediately**.
			//
			// the result is that we WILL NOT wait, therefore fast-forwarding
			// and thus healing the chain by backfilling it with null rounds
			// rapidly.
			deadline := baseTime + buildconstants.PropagationDelaySecs
			baseT := time.Unix(int64(deadline), 0)

			baseT = baseT.Add(randTimeOffset(time.Second))

			build.Clock.Sleep(build.Clock.Until(baseT))

			return func(bool, abi.ChainEpoch, error) {}, 0, nil
		},

		sf:                sf,
		minedBlockHeights: arc,
		evtTypes: [...]journal.EventType{
			evtTypeBlockMined: j.RegisterEventType("miner", "block_mined"),
		},
		journal: j,
	}
}

// Miner encapsulates the mining processes of the system.
//
// Refer to the godocs on mineOne and mine methods for more detail.
type Miner struct {
	api v1api.FullNode

	epp gen.WinningPoStProver

	lk       sync.Mutex
	address  address.Address
	stop     chan struct{}
	stopping chan struct{}

	propagationWaitFunc waitFunc

	// lastWork holds the last MiningBase we built upon.
	lastWork *MiningBase

	sf *slashfilter.SlashFilter
	// minedBlockHeights is a safeguard that caches the last heights we mined.
	// It is consulted before publishing a newly mined block, for a sanity check
	// intended to avoid slashings in case of a bug.
	minedBlockHeights *arc.ARCCache[abi.ChainEpoch, bool]

	evtTypes [1]journal.EventType
	journal  journal.Journal
}

// Address returns the address of the miner.
func (m *Miner) Address() address.Address {
	m.lk.Lock()
	defer m.lk.Unlock()

	return m.address
}

// Start starts the mining operation. It spawns a goroutine and returns
// immediately. Start is not idempotent.
func (m *Miner) Start(_ context.Context) error {
	m.lk.Lock()
	defer m.lk.Unlock()
	if m.stop != nil {
		return fmt.Errorf("miner already started")
	}
	m.stop = make(chan struct{})
	go m.mine(context.TODO())
	return nil
}

// Stop stops the mining operation. It is not idempotent, and multiple adjacent
// calls to Stop will fail.
func (m *Miner) Stop(ctx context.Context) error {
	m.lk.Lock()

	m.stopping = make(chan struct{})
	stopping := m.stopping
	close(m.stop)

	m.lk.Unlock()

	select {
	case <-stopping:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Miner) niceSleep(d time.Duration) bool {
	select {
	case <-build.Clock.After(d):
		return true
	case <-m.stop:
		log.Infow("received interrupt while trying to sleep in mining cycle")
		return false
	}
}

// mine runs the mining loop. It performs the following:
//
//  1. Queries our current best currently-known mining candidate (tipset to
//     build upon).
//  2. Waits until the propagation delay of the network has elapsed (currently
//     6 seconds). The waiting is done relative to the timestamp of the best
//     candidate, which means that if it's way in the past, we won't wait at
//     all (e.g. in catch-up or rush mining).
//  3. After the wait, we query our best mining candidate. This will be the one
//     we'll work with.
//  4. Sanity check that we _actually_ have a new mining base to mine on. If
//     not, wait one epoch + propagation delay, and go back to the top.
//  5. We attempt to mine a block, by calling mineOne (refer to godocs). This
//     method will either return a block if we were eligible to mine, or nil
//     if we weren't.
//     6a. If we mined a block, we update our state and push it out to the network
//     via gossipsub.
//     6b. If we didn't mine a block, we consider this to be a nil round on top of
//     the mining base we selected. If other miners on the network
//     were eligible to mine, we will receive their blocks via gossipsub and
//     we will select that tipset on the next iteration of the loop, thus
//     discarding our null round.
func (m *Miner) mine(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "/mine")
	defer span.End()

	// Perform the Winning PoSt warmup in a separate goroutine.
	go m.doWinPoStWarmup(ctx)

	var lastBase MiningBase

	// Start the main mining loop.
minerLoop:
	for {
		// Prepare a context for a single node operation.
		ctx := cliutil.OnSingleNode(ctx)

		// Handle stop signals.
		select {
		case <-m.stop:
			// If a stop signal is received, clean up and exit the mining loop.
			stopping := m.stopping
			m.stop = nil
			m.stopping = nil
			close(stopping)
			return

		default:
		}

		var base *MiningBase // NOTE: This points to m.lastWork; Incrementing nulls here will increment it there.
		var onDone func(bool, abi.ChainEpoch, error)
		var injectNulls abi.ChainEpoch

		// Look for the best mining candidate.
		for {
			prebase, err := m.GetBestMiningCandidate(ctx)
			if err != nil {
				log.Errorf("failed to get best mining candidate: %s", err)
				if !m.niceSleep(time.Second * 5) {
					continue minerLoop
				}
				continue
			}

			// Check if we have a new base or if the current base is still valid.
			if base != nil && base.TipSet.Height() == prebase.TipSet.Height() && base.NullRounds == prebase.NullRounds {
				base = prebase
				break
			}
			if base != nil {
				onDone(false, 0, nil)
			}

			// TODO: need to change the orchestration here. the problem is that
			// we are waiting *after* we enter this loop and select a mining
			// candidate, which is almost certain to change in multiminer
			// tests. Instead, we should block before entering the loop, so
			// that when the test 'MineOne' function is triggered, we pull our
			// best mining candidate at that time.

			// Wait until propagation delay period after block we plan to mine on
			onDone, injectNulls, err = m.propagationWaitFunc(ctx, prebase.TipSet.MinTimestamp())
			if err != nil {
				log.Error(err)
				continue
			}

			// Ensure the beacon entry is available before finalizing the mining base.
			_, err = m.api.StateGetBeaconEntry(ctx, prebase.TipSet.Height()+prebase.NullRounds+1)
			if err != nil {
				log.Errorf("failed getting beacon entry: %s", err)
				if !m.niceSleep(time.Second) {
					continue minerLoop
				}
				continue
			}

			base = prebase
		}

		base.NullRounds += injectNulls // Adjust for testing purposes.

		// Check for repeated mining candidates and handle sleep for the next round.
		if base.TipSet.Equals(lastBase.TipSet) && lastBase.NullRounds == base.NullRounds {
			log.Warnf("BestMiningCandidate from the previous round: %s (nulls:%d)", lastBase.TipSet.Cids(), lastBase.NullRounds)
			if !m.niceSleep(time.Duration(buildconstants.BlockDelaySecs) * time.Second) {
				continue minerLoop
			}
			continue
		}

		// Attempt to mine a block.
		b, err := m.mineOne(ctx, base)
		if err != nil {
			log.Errorf("mining block failed: %+v", err)
			if !m.niceSleep(time.Second) {
				continue minerLoop
			}
			onDone(false, 0, err)
			continue
		}
		lastBase = *base

		var h abi.ChainEpoch
		if b != nil {
			h = b.Header.Height
		}
		onDone(b != nil, h, nil)

		// Process the mined block.
		switch {
		case b != nil:
			// Record the event of mining a block.
			m.journal.RecordEvent(m.evtTypes[evtTypeBlockMined], func() interface{} {
				return map[string]interface{}{
					// Data about the mined block.
					"parents":   base.TipSet.Cids(),
					"nulls":     base.NullRounds,
					"epoch":     b.Header.Height,
					"timestamp": b.Header.Timestamp,
					"cid":       b.Header.Cid(),
				}
			})

			btime := time.Unix(int64(b.Header.Timestamp), 0)
			now := build.Clock.Now()
			// Handle timing for broadcasting the block.
			switch {
			case btime == now:
				// block timestamp is perfectly aligned with time.
			case btime.After(now):
				// Wait until it's time to broadcast the block.
				if !m.niceSleep(build.Clock.Until(btime)) {
					log.Warnf("received interrupt while waiting to broadcast block, will shutdown after block is sent out")
					build.Clock.Sleep(build.Clock.Until(btime))
				}
			default:
				// Log if the block was mined in the past.
				log.Warnw("mined block in the past",
					"block-time", btime, "time", build.Clock.Now(), "difference", build.Clock.Since(btime))
			}

			// Check for slash filter conditions.
			if os.Getenv("LOTUS_MINER_NO_SLASHFILTER") != "_yes_i_know_i_can_and_probably_will_lose_all_my_fil_and_power_" && !buildconstants.IsNearUpgrade(base.TipSet.Height(), buildconstants.UpgradeWatermelonFixHeight) {
				witness, fault, err := m.sf.MinedBlock(ctx, b.Header, base.TipSet.Height())
				if err != nil {
					log.Errorf("<!!> SLASH FILTER ERRORED: %s", err)
					// Continue here, because it's _probably_ wiser to not submit this block
					break
				}

				if fault {
					log.Errorf("<!!> SLASH FILTER DETECTED FAULT due to blocks %s and %s", b.Header.Cid(), witness)
					break
				}
			}

			// Check for blocks created at the same height.
			if _, ok := m.minedBlockHeights.Get(b.Header.Height); ok {
				log.Warnw("Created a block at the same height as another block we've created", "height", b.Header.Height, "miner", b.Header.Miner, "parents", b.Header.Parents)
				break
			}

			// Add the block height to the mined block heights.
			m.minedBlockHeights.Add(b.Header.Height, true)

			// Submit the newly mined block.
			if err := m.api.SyncSubmitBlock(ctx, b); err != nil {
				log.Errorf("failed to submit newly mined block: %+v", err)
				break
			}
			continue // TODO: we should probably remove this continue and wait in this case as well... but that's a bigger change.
		}

		// If no block was mined or if we fail to submit the block, increase the null rounds and wait for the next epoch.
		base.NullRounds++

		// Calculate the time for the next round.
		nextRound := time.Unix(int64(base.TipSet.MinTimestamp()+buildconstants.BlockDelaySecs*uint64(base.NullRounds))+int64(buildconstants.PropagationDelaySecs), 0)

		// Wait for the next round or stop signal.
		select {
		case <-build.Clock.After(build.Clock.Until(nextRound)):
		case <-m.stop:
			stopping := m.stopping
			m.stop = nil
			m.stopping = nil
			close(stopping)
			return
		}
	}
}

// MiningBase is the tipset on top of which we plan to construct our next block.
// Refer to godocs on GetBestMiningCandidate.
type MiningBase struct {
	TipSet      *types.TipSet
	ComputeTime time.Time
	NullRounds  abi.ChainEpoch
}

// GetBestMiningCandidate implements the fork choice rule from a miner's
// perspective, returning the best head to mine on. This includes the number of null rounds we think
// we should insert and the time at which we received said head.
func (m *Miner) GetBestMiningCandidate(ctx context.Context) (*MiningBase, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	bts, err := m.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	if m.lastWork == nil || !m.lastWork.TipSet.Equals(bts) {
		m.lastWork = &MiningBase{TipSet: bts, ComputeTime: time.Now()}
	}

	return m.lastWork, nil
}

// mineOne attempts to mine a single block, and does so synchronously, if and
// only if we are eligible to mine.
//
// {hint/landmark}: This method coordinates all the steps involved in mining a
// block, including the condition of whether mine or not at all depending on
// whether we win the round or not.
//
// This method does the following:
//
//	1.
func (m *Miner) mineOne(ctx context.Context, base *MiningBase) (minedBlock *types.BlockMsg, err error) {
	log.Debugw("attempting to mine a block", "tipset", types.LogCids(base.TipSet.Cids()))
	tStart := build.Clock.Now()

	round := base.TipSet.Height() + base.NullRounds + 1

	// always write out a log
	var winner *types.ElectionProof
	var mbi *api.MiningBaseInfo
	var rbase types.BeaconEntry
	defer func() {

		var hasMinPower bool

		// mbi can be nil if we are deep in penalty and there are 0 eligible sectors
		// in the current deadline. If this case - put together a dummy one for reporting
		// https://github.com/filecoin-project/lotus/blob/v1.9.0/chain/stmgr/utils.go#L500-L502
		if mbi == nil {
			mbi = &api.MiningBaseInfo{
				NetworkPower:      big.NewInt(-1), // we do not know how big the network is at this point
				EligibleForMining: false,
				MinerPower:        big.NewInt(0), // but we do know we do not have anything eligible
			}

			// try to opportunistically pull actual power and plug it into the fake mbi
			if pow, err := m.api.StateMinerPower(ctx, m.address, base.TipSet.Key()); err == nil && pow != nil {
				hasMinPower = pow.HasMinPower
				mbi.MinerPower = pow.MinerPower.QualityAdjPower
				mbi.NetworkPower = pow.TotalPower.QualityAdjPower
			}
		}

		isLate := uint64(tStart.Unix()) > (base.TipSet.MinTimestamp() + uint64(base.NullRounds*builtin.EpochDurationSeconds) + buildconstants.PropagationDelaySecs)

		logStruct := []interface{}{
			"tookMilliseconds", (build.Clock.Now().UnixNano() - tStart.UnixNano()) / 1_000_000,
			"forRound", int64(round),
			"baseEpoch", int64(base.TipSet.Height()),
			"baseDeltaSeconds", uint64(tStart.Unix()) - base.TipSet.MinTimestamp(),
			"nullRounds", int64(base.NullRounds),
			"lateStart", isLate,
			"beaconEpoch", rbase.Round,
			"lookbackEpochs", int64(policy.ChainFinality), // hardcoded as it is unlikely to change again: https://github.com/filecoin-project/lotus/blob/v1.8.0/chain/actors/policy/policy.go#L180-L186
			"networkPowerAtLookback", mbi.NetworkPower.String(),
			"minerPowerAtLookback", mbi.MinerPower.String(),
			"isEligible", mbi.EligibleForMining,
			"isWinner", (winner != nil),
			"error", err,
		}

		if err != nil {
			log.Errorw("completed mineOne", logStruct...)
		} else if isLate || (hasMinPower && !mbi.EligibleForMining) {
			log.Warnw("completed mineOne", logStruct...)
		} else {
			log.Infow("completed mineOne", logStruct...)
		}
	}()

	mbi, err = m.api.MinerGetBaseInfo(ctx, m.address, round, base.TipSet.Key())
	if err != nil {
		err = xerrors.Errorf("failed to get mining base info: %w", err)
		return nil, err
	}
	if mbi == nil {
		return nil, nil
	}

	if !mbi.EligibleForMining {
		// slashed or just have no power yet
		return nil, nil
	}

	tPowercheck := build.Clock.Now()

	bvals := mbi.BeaconEntries
	rbase = mbi.PrevBeaconEntry
	if len(bvals) > 0 {
		rbase = bvals[len(bvals)-1]
	}

	ticket, err := m.computeTicket(ctx, &rbase, round, base.TipSet.MinTicket(), mbi)
	if err != nil {
		err = xerrors.Errorf("scratching ticket failed: %w", err)
		return nil, err
	}

	winner, err = gen.IsRoundWinner(ctx, round, m.address, rbase, mbi, m.api)
	if err != nil {
		err = xerrors.Errorf("failed to check if we win next round: %w", err)
		return nil, err
	}

	if winner == nil {
		return nil, nil
	}

	tTicket := build.Clock.Now()

	buf := new(bytes.Buffer)
	if err := m.address.MarshalCBOR(buf); err != nil {
		err = xerrors.Errorf("failed to marshal miner address: %w", err)
		return nil, err
	}

	rand, err := lrand.DrawRandomnessFromBase(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, round, buf.Bytes())
	if err != nil {
		err = xerrors.Errorf("failed to get randomness for winning post: %w", err)
		return nil, err
	}

	prand := abi.PoStRandomness(rand)

	tSeed := build.Clock.Now()
	nv, err := m.api.StateNetworkVersion(ctx, base.TipSet.Key())
	if err != nil {
		return nil, err
	}

	postProof, err := m.epp.ComputeProof(ctx, mbi.Sectors, prand, round, nv)
	if err != nil {
		err = xerrors.Errorf("failed to compute winning post proof: %w", err)
		return nil, err
	}

	tProof := build.Clock.Now()

	// get pending messages early,
	msgs, err := m.api.MpoolSelect(ctx, base.TipSet.Key(), ticket.Quality())
	if err != nil {
		err = xerrors.Errorf("failed to select messages for block: %w", err)
		return nil, err
	}

	tPending := build.Clock.Now()

	// This next block exists to "catch" equivocating miners,
	// who submit 2 blocks at the same height at different times in order to split the network.
	// To safeguard against this, we make sure it's been EquivocationDelaySecs since our base was calculated,
	// then re-calculate it.
	// If the daemon detected equivocated blocks, those blocks will no longer be in the new base.
	m.niceSleep(time.Until(base.ComputeTime.Add(time.Duration(buildconstants.EquivocationDelaySecs) * time.Second)))
	newBase, err := m.GetBestMiningCandidate(ctx)
	if err != nil {
		err = xerrors.Errorf("failed to refresh best mining candidate: %w", err)
		return nil, err
	}

	tEquivocateWait := build.Clock.Now()

	// If the base has changed, we take the _intersection_ of our old base and new base,
	// thus ejecting blocks from any equivocating miners, without taking any new blocks.
	if newBase.TipSet.Height() == base.TipSet.Height() && !newBase.TipSet.Equals(base.TipSet) {
		log.Warnf("base changed from %s to %s, taking intersection", base.TipSet.Key(), newBase.TipSet.Key())
		newBaseMap := map[cid.Cid]struct{}{}
		for _, newBaseBlk := range newBase.TipSet.Cids() {
			newBaseMap[newBaseBlk] = struct{}{}
		}

		refreshedBaseBlocks := make([]*types.BlockHeader, 0, len(base.TipSet.Cids()))
		for _, baseBlk := range base.TipSet.Blocks() {
			if _, ok := newBaseMap[baseBlk.Cid()]; ok {
				refreshedBaseBlocks = append(refreshedBaseBlocks, baseBlk)
			}
		}

		if len(refreshedBaseBlocks) != 0 && len(refreshedBaseBlocks) != len(base.TipSet.Blocks()) {
			refreshedBase, err := types.NewTipSet(refreshedBaseBlocks)
			if err != nil {
				err = xerrors.Errorf("failed to create new tipset when refreshing: %w", err)
				return nil, err
			}

			if !base.TipSet.MinTicket().Equals(refreshedBase.MinTicket()) {
				log.Warn("recomputing ticket due to base refresh")

				ticket, err = m.computeTicket(ctx, &rbase, round, refreshedBase.MinTicket(), mbi)
				if err != nil {
					err = xerrors.Errorf("failed to refresh ticket: %w", err)
					return nil, err
				}
			}

			log.Warn("re-selecting messages due to base refresh")
			// refresh messages, as the selected messages may no longer be valid
			msgs, err = m.api.MpoolSelect(ctx, refreshedBase.Key(), ticket.Quality())
			if err != nil {
				err = xerrors.Errorf("failed to re-select messages for block: %w", err)
				return nil, err
			}

			base.TipSet = refreshedBase
		}
	}

	tIntersectAndRefresh := build.Clock.Now()

	// TODO: winning post proof
	minedBlock, err = m.createBlock(base, m.address, ticket, winner, bvals, postProof, msgs)
	if err != nil {
		err = xerrors.Errorf("failed to create block: %w", err)
		return nil, err
	}

	tCreateBlock := build.Clock.Now()
	dur := tCreateBlock.Sub(tStart)
	parentMiners := make([]address.Address, len(base.TipSet.Blocks()))
	for i, header := range base.TipSet.Blocks() {
		parentMiners[i] = header.Miner
	}
	log.Infow("mined new block", "cid", minedBlock.Cid(), "height", int64(minedBlock.Header.Height), "miner", minedBlock.Header.Miner, "parents", parentMiners, "parentTipset", base.TipSet.Key().String(), "took", dur)
	if dur > time.Second*time.Duration(buildconstants.BlockDelaySecs) || time.Now().Compare(time.Unix(int64(minedBlock.Header.Timestamp), 0)) >= 0 {
		log.Warnw("CAUTION: block production took us past the block time. Your computer may not be fast enough to keep up",
			"tPowercheck ", tPowercheck.Sub(tStart),
			"tTicket ", tTicket.Sub(tPowercheck),
			"tSeed ", tSeed.Sub(tTicket),
			"tProof ", tProof.Sub(tSeed),
			"tPending ", tPending.Sub(tProof),
			"tEquivocateWait ", tEquivocateWait.Sub(tPending),
			"tIntersectAndRefresh ", tIntersectAndRefresh.Sub(tEquivocateWait),
			"tCreateBlock ", tCreateBlock.Sub(tIntersectAndRefresh))
	}

	return minedBlock, nil
}

func (m *Miner) computeTicket(ctx context.Context, brand *types.BeaconEntry, round abi.ChainEpoch, chainRand *types.Ticket, mbi *api.MiningBaseInfo) (*types.Ticket, error) {
	buf := new(bytes.Buffer)
	if err := m.address.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal address to cbor: %w", err)
	}

	if round > buildconstants.UpgradeSmokeHeight {
		buf.Write(chainRand.VRFProof)
	}

	input, err := lrand.DrawRandomnessFromBase(brand.Data, crypto.DomainSeparationTag_TicketProduction, round-buildconstants.TicketRandomnessLookback, buf.Bytes())
	if err != nil {
		return nil, err
	}

	vrfOut, err := gen.ComputeVRF(ctx, m.api.WalletSign, mbi.WorkerKey, input)
	if err != nil {
		return nil, err
	}

	return &types.Ticket{
		VRFProof: vrfOut,
	}, nil
}

func (m *Miner) createBlock(base *MiningBase, addr address.Address, ticket *types.Ticket,
	eproof *types.ElectionProof, bvals []types.BeaconEntry, wpostProof []proof.PoStProof, msgs []*types.SignedMessage) (*types.BlockMsg, error) {
	uts := base.TipSet.MinTimestamp() + buildconstants.BlockDelaySecs*(uint64(base.NullRounds)+1)

	nheight := base.TipSet.Height() + base.NullRounds + 1

	// why even return this? that api call could just submit it for us
	return m.api.MinerCreateBlock(context.TODO(), &api.BlockTemplate{
		Miner:            addr,
		Parents:          base.TipSet.Key(),
		Ticket:           ticket,
		Eproof:           eproof,
		BeaconValues:     bvals,
		Messages:         msgs,
		Epoch:            nheight,
		Timestamp:        uts,
		WinningPoStProof: wpostProof,
	})
}
