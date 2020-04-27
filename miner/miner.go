package miner

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	address "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	lru "github.com/hashicorp/golang-lru"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"

	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"
)

var log = logging.Logger("miner")

// returns a callback reporting whether we mined a blocks in this round
type waitFunc func(ctx context.Context, baseTime uint64) (func(bool), error)

func NewMiner(api api.FullNode, epp gen.WinningPoStProver, beacon beacon.RandomBeacon) *Miner {
	arc, err := lru.NewARC(10000)
	if err != nil {
		panic(err)
	}

	return &Miner{
		api:    api,
		epp:    epp,
		beacon: beacon,
		waitFunc: func(ctx context.Context, baseTime uint64) (func(bool), error) {
			// Wait around for half the block time in case other parents come in
			deadline := baseTime + build.PropagationDelay
			time.Sleep(time.Until(time.Unix(int64(deadline), 0)))

			return func(bool) {}, nil
		},
		minedBlockHeights: arc,
	}
}

type Miner struct {
	api api.FullNode

	epp    gen.WinningPoStProver
	beacon beacon.RandomBeacon

	lk        sync.Mutex
	addresses []address.Address
	stop      chan struct{}
	stopping  chan struct{}

	waitFunc waitFunc

	lastWork *MiningBase

	minedBlockHeights *lru.ARCCache
}

func (m *Miner) Addresses() ([]address.Address, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	out := make([]address.Address, len(m.addresses))
	copy(out, m.addresses)

	return out, nil
}

func (m *Miner) Register(addr address.Address) error {
	m.lk.Lock()
	defer m.lk.Unlock()

	if len(m.addresses) > 0 {
		for _, a := range m.addresses {
			if a == addr {
				log.Warnf("miner.Register called more than once for actor '%s'", addr)
				return xerrors.Errorf("miner.Register called more than once for actor '%s'", addr)
			}
		}
	}

	m.addresses = append(m.addresses, addr)
	if len(m.addresses) == 1 {
		m.stop = make(chan struct{})
		go m.mine(context.TODO())
	}

	return nil
}

func (m *Miner) Unregister(ctx context.Context, addr address.Address) error {
	m.lk.Lock()
	defer m.lk.Unlock()
	if len(m.addresses) == 0 {
		return xerrors.New("no addresses registered")
	}

	idx := -1

	for i, a := range m.addresses {
		if a == addr {
			idx = i
			break
		}
	}
	if idx == -1 {
		return xerrors.New("unregister: address not found")
	}

	m.addresses[idx] = m.addresses[len(m.addresses)-1]
	m.addresses = m.addresses[:len(m.addresses)-1]

	// Unregistering last address, stop mining first
	if len(m.addresses) == 0 && m.stop != nil {
		m.stopping = make(chan struct{})
		stopping := m.stopping
		close(m.stop)

		select {
		case <-stopping:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (m *Miner) niceSleep(d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-m.stop:
		return false
	}
}

func (m *Miner) mine(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "/mine")
	defer span.End()

	var lastBase MiningBase

eventLoop:
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

		m.lk.Lock()
		addrs := m.addresses
		m.lk.Unlock()

		prebase, err := m.GetBestMiningCandidate(ctx)
		if err != nil {
			log.Errorf("failed to get best mining candidate: %s", err)
			m.niceSleep(time.Second * 5)
			continue
		}

		// Wait until propagation delay period after block we plan to mine on
		onDone, err := m.waitFunc(ctx, prebase.TipSet.MinTimestamp())
		if err != nil {
			log.Error(err)
			return
		}

		base, err := m.GetBestMiningCandidate(ctx)
		if err != nil {
			log.Errorf("failed to get best mining candidate: %s", err)
			continue
		}
		if base.TipSet.Equals(lastBase.TipSet) && lastBase.NullRounds == base.NullRounds {
			log.Warnf("BestMiningCandidate from the previous round: %s (nulls:%d)", lastBase.TipSet.Cids(), lastBase.NullRounds)
			m.niceSleep(build.BlockDelay * time.Second)
			continue
		}
		lastBase = *base

		blks := make([]*types.BlockMsg, 0)

		for _, addr := range addrs {
			b, err := m.mineOne(ctx, addr, base)
			if err != nil {
				log.Errorf("mining block failed: %+v", err)
				continue
			}
			if b != nil {
				blks = append(blks, b)
			}
		}

		onDone(len(blks) != 0)

		if len(blks) != 0 {
			btime := time.Unix(int64(blks[0].Header.Timestamp), 0)
			if time.Now().Before(btime) {
				if !m.niceSleep(time.Until(btime)) {
					log.Warnf("received interrupt while waiting to broadcast block, will shutdown after block is sent out")
					time.Sleep(time.Until(btime))
				}
			} else {
				log.Warnw("mined block in the past", "block-time", btime,
					"time", time.Now(), "duration", time.Since(btime))
			}

			mWon := make(map[address.Address]struct{})
			for _, b := range blks {
				_, notOk := mWon[b.Header.Miner]
				if notOk {
					log.Errorw("2 blocks for the same miner. Throwing hands in the air. Report this. It is important.", "blocks", blks)
					continue eventLoop
				}
				mWon[b.Header.Miner] = struct{}{}
			}
			for _, b := range blks {
				// TODO: this code was written to handle creating blocks for multiple miners.
				// However, we don't use that, and we probably never will. So even though this code will
				// never see different miners, i'm going to handle the caching as if it was going to.
				// We can clean it up later when we remove all the multiple miner logic.
				blkKey := fmt.Sprintf("%s-%d", b.Header.Miner, b.Header.Height)
				if _, ok := m.minedBlockHeights.Get(blkKey); ok {
					log.Warnw("Created a block at the same height as another block we've created", "height", b.Header.Height, "miner", b.Header.Miner, "parents", b.Header.Parents)
					continue
				}

				m.minedBlockHeights.Add(blkKey, true)
				if err := m.api.SyncSubmitBlock(ctx, b); err != nil {
					log.Errorf("failed to submit newly mined block: %s", err)
				}
			}
		} else {
			nextRound := time.Unix(int64(base.TipSet.MinTimestamp()+uint64(build.BlockDelay*base.NullRounds)), 0)

			select {
			case <-time.After(time.Until(nextRound)):
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

type MiningBase struct {
	TipSet     *types.TipSet
	NullRounds abi.ChainEpoch
}

func (m *Miner) GetBestMiningCandidate(ctx context.Context) (*MiningBase, error) {
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
			return nil, err
		}

		if types.BigCmp(btsw, ltsw) <= 0 {
			return m.lastWork, nil
		}
	}

	m.lastWork = &MiningBase{TipSet: bts}
	return m.lastWork, nil
}

func (m *Miner) hasPower(ctx context.Context, addr address.Address, ts *types.TipSet) (bool, error) {
	power, err := m.api.StateMinerPower(ctx, addr, ts.Key())
	if err != nil {
		return false, err
	}

	return !power.MinerPower.QualityAdjPower.Equals(types.NewInt(0)), nil
}

func (m *Miner) mineOne(ctx context.Context, addr address.Address, base *MiningBase) (*types.BlockMsg, error) {
	log.Debugw("attempting to mine a block", "tipset", types.LogCids(base.TipSet.Cids()))
	start := time.Now()

	round := base.TipSet.Height() + base.NullRounds + 1

	mbi, err := m.api.MinerGetBaseInfo(ctx, addr, round, base.TipSet.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get mining base info: %w", err)
	}
	if mbi == nil {
		base.NullRounds++
		return nil, nil
	}

	beaconPrev := mbi.PrevBeaconEntry

	bvals, err := beacon.BeaconEntriesForBlock(ctx, m.beacon, round, beaconPrev)
	if err != nil {
		return nil, xerrors.Errorf("get beacon entries failed: %w", err)
	}

	hasPower, err := m.hasPower(ctx, addr, base.TipSet)
	if err != nil {
		return nil, xerrors.Errorf("checking if miner is slashed: %w", err)
	}
	if !hasPower {
		// slashed or just have no power yet
		base.NullRounds++
		return nil, nil
	}

	log.Infof("Time delta between now and our mining base: %ds (nulls: %d)", uint64(time.Now().Unix())-base.TipSet.MinTimestamp(), base.NullRounds)

	rbase := beaconPrev
	if len(bvals) > 0 {
		rbase = bvals[len(bvals)-1]
	}

	ticket, err := m.computeTicket(ctx, addr, &rbase, base)
	if err != nil {
		return nil, xerrors.Errorf("scratching ticket failed: %w", err)
	}

	winner, err := gen.IsRoundWinner(ctx, base.TipSet, round, addr, rbase, mbi, m.api)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if we win next round: %w", err)
	}

	if winner == nil {
		base.NullRounds++
		return nil, nil
	}

	rand, err := m.api.ChainGetRandomness(ctx, base.TipSet.Key(), crypto.DomainSeparationTag_WinningPoStChallengeSeed, base.TipSet.Height()+base.NullRounds, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to get randomness for winning post: %w", err)
	}

	prand := abi.PoStRandomness(rand)

	postProof, err := m.epp.ComputeProof(ctx, mbi.Sectors, prand)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute winning post proof: %w", err)
	}

	// get pending messages early,
	pending, err := m.api.MpoolPending(context.TODO(), base.TipSet.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get pending messages: %w", err)
	}

	// TODO: winning post proof
	b, err := m.createBlock(base, addr, ticket, winner, bvals, postProof, pending)
	if err != nil {
		return nil, xerrors.Errorf("failed to create block: %w", err)
	}

	dur := time.Since(start)
	log.Infow("mined new block", "cid", b.Cid(), "height", b.Header.Height, "took", dur)
	if dur > time.Second*build.BlockDelay {
		log.Warn("CAUTION: block production took longer than the block delay. Your computer may not be fast enough to keep up")
	}

	return b, nil
}

func (m *Miner) computeTicket(ctx context.Context, addr address.Address, brand *types.BeaconEntry, base *MiningBase) (*types.Ticket, error) {
	mi, err := m.api.StateMinerInfo(ctx, addr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	worker, err := m.api.StateAccountKey(ctx, mi.Worker, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err := addr.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal address to cbor: %w", err)
	}
	input, err := m.api.ChainGetRandomness(ctx, base.TipSet.Key(), crypto.DomainSeparationTag_TicketProduction,
		base.TipSet.Height()+base.NullRounds+1-build.TicketRandomnessLookback, buf.Bytes())
	if err != nil {
		return nil, err
	}

	vrfOut, err := gen.ComputeVRF(ctx, m.api.WalletSign, worker, input)
	if err != nil {
		return nil, err
	}

	return &types.Ticket{
		VRFProof: vrfOut,
	}, nil
}

func (m *Miner) createBlock(base *MiningBase, addr address.Address, ticket *types.Ticket,
	eproof *types.ElectionProof, bvals []types.BeaconEntry, wpostProof []abi.PoStProof, pending []*types.SignedMessage) (*types.BlockMsg, error) {
	msgs, err := SelectMessages(context.TODO(), m.api.StateGetActor, base.TipSet, pending)
	if err != nil {
		return nil, xerrors.Errorf("message filtering failed: %w", err)
	}

	if len(msgs) > build.BlockMessageLimit {
		log.Error("SelectMessages returned too many messages: ", len(msgs))
		msgs = msgs[:build.BlockMessageLimit]
	}

	uts := base.TipSet.MinTimestamp() + uint64(build.BlockDelay*(base.NullRounds+1))

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

type ActorLookup func(context.Context, address.Address, types.TipSetKey) (*types.Actor, error)

func countFrom(msgs []*types.SignedMessage, from address.Address) (out int) {
	for _, msg := range msgs {
		if msg.Message.From == from {
			out++
		}
	}
	return out
}

func SelectMessages(ctx context.Context, al ActorLookup, ts *types.TipSet, msgs []*types.SignedMessage) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, 0, build.BlockMessageLimit)
	inclNonces := make(map[address.Address]uint64)
	inclBalances := make(map[address.Address]types.BigInt)
	inclCount := make(map[address.Address]int)

	for _, msg := range msgs {

		if msg.Message.To == address.Undef {
			log.Warnf("message in mempool had bad 'To' address")
			continue
		}

		from := msg.Message.From

		if _, ok := inclNonces[from]; !ok {
			act, err := al(ctx, from, ts.Key())
			if err != nil {
				log.Warnf("failed to check message sender balance, skipping message: %+v", err)
				continue
			}

			inclNonces[from] = act.Nonce
			inclBalances[from] = act.Balance
		}

		if inclBalances[from].LessThan(msg.Message.RequiredFunds()) {
			log.Warnf("message in mempool does not have enough funds: %s", msg.Cid())
			continue
		}

		if msg.Message.Nonce > inclNonces[from] {
			log.Debugf("message in mempool has too high of a nonce (%d > %d, from %s, inclcount %d) %s (%d pending for orig)", msg.Message.Nonce, inclNonces[from], from, inclCount[from], msg.Cid(), countFrom(msgs, from))
			continue
		}

		if msg.Message.Nonce < inclNonces[from] {
			log.Warnf("message in mempool has already used nonce (%d < %d), from %s, to %s, %s (%d pending for)", msg.Message.Nonce, inclNonces[from], msg.Message.From, msg.Message.To, msg.Cid(), countFrom(msgs, from))
			continue
		}

		inclNonces[from] = msg.Message.Nonce + 1
		inclBalances[from] = types.BigSub(inclBalances[from], msg.Message.RequiredFunds())
		inclCount[from]++

		out = append(out, msg)
		if len(out) >= build.BlockMessageLimit {
			break
		}
	}
	return out, nil
}
