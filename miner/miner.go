package miner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/api/v1api"

	proof2 "github.com/filecoin-project/specs-actors/v2/actors/runtime/proof"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	lru "github.com/hashicorp/golang-lru"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/journal"

	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"
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
	rand.Reader.Read(buf) //nolint:errcheck
	val := time.Duration(binary.BigEndian.Uint64(buf) % uint64(width))

	return val - (width / 2)
}

// NewMiner instantiates a miner with a concrete WinningPoStProver and a miner
// address (which can be different from the worker's address).
func NewMiner(api v1api.FullNode, epp gen.WinningPoStProver, addr address.Address, sf *slashfilter.SlashFilter, j journal.Journal) *Miner {
	arc, err := lru.NewARC(10000)
	if err != nil {
		panic(err)
	}

	return &Miner{
		api:     api,
		epp:     epp,
		address: addr,
		waitFunc: func(ctx context.Context, baseTime uint64) (func(bool, abi.ChainEpoch, error), abi.ChainEpoch, error) {
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
			deadline := baseTime + build.PropagationDelaySecs
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

	waitFunc waitFunc

	// lastWork holds the last MiningBase we built upon.
	lastWork *MiningBase

	sf *slashfilter.SlashFilter
	// minedBlockHeights is a safeguard that caches the last heights we mined.
	// It is consulted before publishing a newly mined block, for a sanity check
	// intended to avoid slashings in case of a bug.
	minedBlockHeights *lru.ARCCache

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
//  1.  Queries our current best currently-known mining candidate (tipset to
//      build upon).
//  2.  Waits until the propagation delay of the network has elapsed (currently
//      6 seconds). The waiting is done relative to the timestamp of the best
//      candidate, which means that if it's way in the past, we won't wait at
//      all (e.g. in catch-up or rush mining).
//  3.  After the wait, we query our best mining candidate. This will be the one
//      we'll work with.
//  4.  Sanity check that we _actually_ have a new mining base to mine on. If
//      not, wait one epoch + propagation delay, and go back to the top.
//  5.  We attempt to mine a block, by calling mineOne (refer to godocs). This
//      method will either return a block if we were eligible to mine, or nil
//      if we weren't.
//  6a. If we mined a block, we update our state and push it out to the network
//      via gossipsub.
//  6b. If we didn't mine a block, we consider this to be a nil round on top of
//      the mining base we selected. If other miner or miners on the network
//      were eligible to mine, we will receive their blocks via gossipsub and
//      we will select that tipset on the next iteration of the loop, thus
//      discarding our null round.
func (m *Miner) mine(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "/mine")
	defer span.End()

	go m.doWinPoStWarmup(ctx)

	var lastBase MiningBase
minerLoop:
	for {
		select {
		case <-m.stop:
			stopping := m.stopping
			m.stop = nil
			m.stopping = nil
			close(stopping)
			return

		default:
		}

		var base *MiningBase
		var onDone func(bool, abi.ChainEpoch, error)
		var injectNulls abi.ChainEpoch

		for {
			prebase, err := m.GetBestMiningCandidate(ctx)
			if err != nil {
				log.Errorf("failed to get best mining candidate: %s", err)
				if !m.niceSleep(time.Second * 5) {
					continue minerLoop
				}
				continue
			}

			if base != nil && base.TipSet.Height() == prebase.TipSet.Height() && base.NullRounds == prebase.NullRounds {
				base = prebase
				break
			}
			if base != nil {
				onDone(false, 0, nil)
			}

			// TODO: need to change the orchestration here. the problem is that
			// we are waiting *after* we enter this loop and selecta mining
			// candidate, which is almost certain to change in multiminer
			// tests. Instead, we should block before entering the loop, so
			// that when the test 'MineOne' function is triggered, we pull our
			// best mining candidate at that time.

			// Wait until propagation delay period after block we plan to mine on
			onDone, injectNulls, err = m.waitFunc(ctx, prebase.TipSet.MinTimestamp())
			if err != nil {
				log.Error(err)
				continue
			}

			// just wait for the beacon entry to become available before we select our final mining base
			_, err = m.api.BeaconGetEntry(ctx, prebase.TipSet.Height()+prebase.NullRounds+1)
			if err != nil {
				log.Errorf("failed getting beacon entry: %s", err)
				if !m.niceSleep(time.Second) {
					continue minerLoop
				}
				continue
			}

			base = prebase
		}

		base.NullRounds += injectNulls // testing

		if base.TipSet.Equals(lastBase.TipSet) && lastBase.NullRounds == base.NullRounds {
			log.Warnf("BestMiningCandidate from the previous round: %s (nulls:%d)", lastBase.TipSet.Cids(), lastBase.NullRounds)
			if !m.niceSleep(time.Duration(build.BlockDelaySecs) * time.Second) {
				continue minerLoop
			}
			continue
		}

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

		if b != nil {
			m.journal.RecordEvent(m.evtTypes[evtTypeBlockMined], func() interface{} {
				return map[string]interface{}{
					"parents":   base.TipSet.Cids(),
					"nulls":     base.NullRounds,
					"epoch":     b.Header.Height,
					"timestamp": b.Header.Timestamp,
					"cid":       b.Header.Cid(),
				}
			})

			btime := time.Unix(int64(b.Header.Timestamp), 0)
			now := build.Clock.Now()
			switch {
			case btime == now:
				// block timestamp is perfectly aligned with time.
			case btime.After(now):
				if !m.niceSleep(build.Clock.Until(btime)) {
					log.Warnf("received interrupt while waiting to broadcast block, will shutdown after block is sent out")
					build.Clock.Sleep(build.Clock.Until(btime))
				}
			default:
				log.Warnw("mined block in the past",
					"block-time", btime, "time", build.Clock.Now(), "difference", build.Clock.Since(btime))
			}

			if err := m.sf.MinedBlock(b.Header, base.TipSet.Height()+base.NullRounds); err != nil {
				log.Errorf("<!!> SLASH FILTER ERROR: %s", err)
				continue
			}

			blkKey := fmt.Sprintf("%d", b.Header.Height)
			if _, ok := m.minedBlockHeights.Get(blkKey); ok {
				log.Warnw("Created a block at the same height as another block we've created", "height", b.Header.Height, "miner", b.Header.Miner, "parents", b.Header.Parents)
				continue
			}

			m.minedBlockHeights.Add(blkKey, true)

			if err := m.api.SyncSubmitBlock(ctx, b); err != nil {
				log.Errorf("failed to submit newly mined block: %+v", err)
			}
		} else {
			base.NullRounds++

			// Wait until the next epoch, plus the propagation delay, so a new tipset
			// has enough time to form.
			//
			// See:  https://github.com/filecoin-project/lotus/issues/1845
			nextRound := time.Unix(int64(base.TipSet.MinTimestamp()+build.BlockDelaySecs*uint64(base.NullRounds))+int64(build.PropagationDelaySecs), 0)

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
}

// MiningBase is the tipset on top of which we plan to construct our next block.
// Refer to godocs on GetBestMiningCandidate.
type MiningBase struct {
	TipSet     *types.TipSet
	NullRounds abi.ChainEpoch
}

// GetBestMiningCandidate implements the fork choice rule from a miner's
// perspective.
//
// It obtains the current chain head (HEAD), and compares it to the last tipset
// we selected as our mining base (LAST). If HEAD's weight is larger than
// LAST's weight, it selects HEAD to build on. Else, it selects LAST.
func (m *Miner) GetBestMiningCandidate(ctx context.Context) (*MiningBase, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	bts, err := m.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	if m.lastWork != nil {
		if m.lastWork.TipSet.Equals(bts) {
			return m.lastWork, nil
		}

		btsw, err := m.api.ChainTipSetWeight(ctx, bts.Key())
		if err != nil {
			return nil, err
		}
		ltsw, err := m.api.ChainTipSetWeight(ctx, m.lastWork.TipSet.Key())
		if err != nil {
			m.lastWork = nil
			return nil, err
		}

		if types.BigCmp(btsw, ltsw) <= 0 {
			return m.lastWork, nil
		}
	}

	m.lastWork = &MiningBase{TipSet: bts}
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
//  1.
func (m *Miner) mineOne(ctx context.Context, base *MiningBase) (*types.BlockMsg, error) {
	log.Debugw("attempting to mine a block", "tipset", types.LogCids(base.TipSet.Cids()))
	start := build.Clock.Now()

	round := base.TipSet.Height() + base.NullRounds + 1

	mbi, err := m.api.MinerGetBaseInfo(ctx, m.address, round, base.TipSet.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get mining base info: %w", err)
	}
	if mbi == nil {
		log.Warnf("mineOne: unexpectedly nil MiningBaseInfo for round %d, off tipset %d/%s", round, base.TipSet.Height(), base.TipSet.Key().String())
		return nil, nil
	}

	// always write out a log from this point out
	var winner *types.ElectionProof
	defer func() {
		log.Infow(
			"completed mineOne",
			"forRound", int64(round),
			"baseEpoch", int64(base.TipSet.Height()),
			"lookbackEpochs", int64(policy.ChainFinality), // hardcoded as it is unlikely to change again: https://github.com/filecoin-project/lotus/blob/v1.8.0/chain/actors/policy/policy.go#L180-L186
			"networkPowerAtLookback", mbi.NetworkPower.String(),
			"minerPowerAtLookback", mbi.MinerPower.String(),
			"isEligible", mbi.EligibleForMining,
			"isWinner", (winner != nil),
		)
	}()

	if !mbi.EligibleForMining {
		// slashed or just have no power yet
		return nil, nil
	}

	tMBI := build.Clock.Now()

	beaconPrev := mbi.PrevBeaconEntry

	tDrand := build.Clock.Now()
	bvals := mbi.BeaconEntries

	tPowercheck := build.Clock.Now()

	log.Infof("Time delta between now and our mining base: %ds (nulls: %d)", uint64(build.Clock.Now().Unix())-base.TipSet.MinTimestamp(), base.NullRounds)

	rbase := beaconPrev
	if len(bvals) > 0 {
		rbase = bvals[len(bvals)-1]
	}

	ticket, err := m.computeTicket(ctx, &rbase, base, mbi)
	if err != nil {
		return nil, xerrors.Errorf("scratching ticket failed: %w", err)
	}

	winner, err = gen.IsRoundWinner(ctx, base.TipSet, round, m.address, rbase, mbi, m.api)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if we win next round: %w", err)
	}

	if winner == nil {
		return nil, nil
	}

	tTicket := build.Clock.Now()

	buf := new(bytes.Buffer)
	if err := m.address.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal miner address: %w", err)
	}

	rand, err := store.DrawRandomness(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, round, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to get randomness for winning post: %w", err)
	}

	prand := abi.PoStRandomness(rand)

	tSeed := build.Clock.Now()

	postProof, err := m.epp.ComputeProof(ctx, mbi.Sectors, prand)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute winning post proof: %w", err)
	}

	tProof := build.Clock.Now()

	// get pending messages early,
	msgs, err := m.api.MpoolSelect(context.TODO(), base.TipSet.Key(), ticket.Quality())
	if err != nil {
		return nil, xerrors.Errorf("failed to select messages for block: %w", err)
	}

	tPending := build.Clock.Now()

	// TODO: winning post proof
	b, err := m.createBlock(base, m.address, ticket, winner, bvals, postProof, msgs)
	if err != nil {
		return nil, xerrors.Errorf("failed to create block: %w", err)
	}

	tCreateBlock := build.Clock.Now()
	dur := tCreateBlock.Sub(start)
	parentMiners := make([]address.Address, len(base.TipSet.Blocks()))
	for i, header := range base.TipSet.Blocks() {
		parentMiners[i] = header.Miner
	}
	log.Infow("mined new block", "cid", b.Cid(), "height", int64(b.Header.Height), "miner", b.Header.Miner, "parents", parentMiners, "parentTipset", base.TipSet.Key().String(), "took", dur)
	if dur > time.Second*time.Duration(build.BlockDelaySecs) {
		log.Warnw("CAUTION: block production took longer than the block delay. Your computer may not be fast enough to keep up",
			"tMinerBaseInfo ", tMBI.Sub(start),
			"tDrand ", tDrand.Sub(tMBI),
			"tPowercheck ", tPowercheck.Sub(tDrand),
			"tTicket ", tTicket.Sub(tPowercheck),
			"tSeed ", tSeed.Sub(tTicket),
			"tProof ", tProof.Sub(tSeed),
			"tPending ", tPending.Sub(tProof),
			"tCreateBlock ", tCreateBlock.Sub(tPending))
	}

	return b, nil
}

func (m *Miner) computeTicket(ctx context.Context, brand *types.BeaconEntry, base *MiningBase, mbi *api.MiningBaseInfo) (*types.Ticket, error) {
	buf := new(bytes.Buffer)
	if err := m.address.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal address to cbor: %w", err)
	}

	round := base.TipSet.Height() + base.NullRounds + 1
	if round > build.UpgradeSmokeHeight {
		buf.Write(base.TipSet.MinTicket().VRFProof)
	}

	input, err := store.DrawRandomness(brand.Data, crypto.DomainSeparationTag_TicketProduction, round-build.TicketRandomnessLookback, buf.Bytes())
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
	eproof *types.ElectionProof, bvals []types.BeaconEntry, wpostProof []proof2.PoStProof, msgs []*types.SignedMessage) (*types.BlockMsg, error) {
	uts := base.TipSet.MinTimestamp() + build.BlockDelaySecs*(uint64(base.NullRounds)+1)

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
