package miner

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"

	logging "github.com/ipfs/go-log"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"
)

var log = logging.Logger("miner")

type waitFunc func(ctx context.Context) error

func NewMiner(api api.FullNode, epp gen.ElectionPoStProver) *Miner {
	return &Miner{
		api: api,
		epp: epp,
		waitFunc: func(ctx context.Context) error {
			// Wait around for half the block time in case other parents come in
			time.Sleep(build.PropagationDelay * time.Second)
			return nil
		},
	}
}

type Miner struct {
	api api.FullNode

	epp gen.ElectionPoStProver

	lk        sync.Mutex
	addresses []address.Address
	stop      chan struct{}
	stopping  chan struct{}

	waitFunc waitFunc

	lastWork *MiningBase
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

		// Sleep a small amount in order to wait for other blocks to arrive
		if err := m.waitFunc(ctx); err != nil {
			log.Error(err)
			return
		}

		base, err := m.GetBestMiningCandidate(ctx)
		if err != nil {
			log.Errorf("failed to get best mining candidate: %s", err)
			continue
		}
		if base.ts.Equals(lastBase.ts) && lastBase.nullRounds == base.nullRounds {
			log.Warnf("BestMiningCandidate from the previous round: %s (nulls:%d)", lastBase.ts.Cids(), lastBase.nullRounds)
			time.Sleep(build.BlockDelay * time.Second)
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

		if len(blks) != 0 {
			btime := time.Unix(int64(blks[0].Header.Timestamp), 0)
			if time.Now().Before(btime) {
				time.Sleep(time.Until(btime))
			} else {
				log.Warnw("mined block in the past", "block-time", btime,
					"time", time.Now(), "duration", time.Now().Sub(btime))
			}

			mWon := make(map[address.Address]struct{})
			for _, b := range blks {
				_, notOk := mWon[b.Header.Miner]
				if notOk {
					log.Errorw("2 blocks for the same miner. Throwing hands in the air. Report this. It is important.", "bloks", blks)
					continue eventLoop
				}
				mWon[b.Header.Miner] = struct{}{}
			}
			for _, b := range blks {
				if err := m.api.SyncSubmitBlock(ctx, b); err != nil {
					log.Errorf("failed to submit newly mined block: %s", err)
				}
			}
		} else {
			nextRound := time.Unix(int64(base.ts.MinTimestamp()+uint64(build.BlockDelay*base.nullRounds)), 0)
			time.Sleep(time.Until(nextRound))
		}
	}
}

type MiningBase struct {
	ts         *types.TipSet
	nullRounds uint64
}

func (m *Miner) GetBestMiningCandidate(ctx context.Context) (*MiningBase, error) {
	bts, err := m.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	if m.lastWork != nil {
		if m.lastWork.ts.Equals(bts) {
			return m.lastWork, nil
		}

		btsw, err := m.api.ChainTipSetWeight(ctx, bts)
		if err != nil {
			return nil, err
		}
		ltsw, err := m.api.ChainTipSetWeight(ctx, m.lastWork.ts)
		if err != nil {
			return nil, err
		}

		if types.BigCmp(btsw, ltsw) <= 0 {
			return m.lastWork, nil
		}
	}

	m.lastWork = &MiningBase{ts: bts}
	return m.lastWork, nil
}

func (m *Miner) hasPower(ctx context.Context, addr address.Address, ts *types.TipSet) (bool, error) {
	power, err := m.api.StateMinerPower(ctx, addr, ts)
	if err != nil {
		return false, err
	}

	return power.MinerPower.Equals(types.NewInt(0)), nil
}

func (m *Miner) mineOne(ctx context.Context, addr address.Address, base *MiningBase) (*types.BlockMsg, error) {
	log.Debugw("attempting to mine a block", "tipset", types.LogCids(base.ts.Cids()))
	start := time.Now()

	hasPower, err := m.hasPower(ctx, addr, base.ts)
	if err != nil {
		return nil, xerrors.Errorf("checking if miner is slashed: %w", err)
	}
	if hasPower {
		// slashed or just have no power yet
		base.nullRounds++
		return nil, nil
	}

	ticket, err := m.computeTicket(ctx, addr, base)
	if err != nil {
		return nil, xerrors.Errorf("scratching ticket failed: %w", err)
	}

	win, proofin, err := gen.IsRoundWinner(ctx, base.ts, int64(base.ts.Height()+base.nullRounds+1), addr, m.epp, m.api)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if we win next round: %w", err)
	}

	if !win {
		base.nullRounds++
		return nil, nil
	}

	// get pending messages early,
	pending, err := m.api.MpoolPending(context.TODO(), base.ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get pending messages: %w", err)
	}

	proof, err := gen.ComputeProof(ctx, m.epp, proofin)
	if err != nil {
		return nil, xerrors.Errorf("computing election proof: %w", err)
	}

	b, err := m.createBlock(base, addr, ticket, proof, pending)
	if err != nil {
		return nil, xerrors.Errorf("failed to create block: %w", err)
	}
	log.Infow("mined new block", "cid", b.Cid(), "height", b.Header.Height)

	dur := time.Now().Sub(start)
	log.Infof("Creating block took %s", dur)
	if dur > time.Second*build.BlockDelay {
		log.Warn("CAUTION: block production took longer than the block delay. Your computer may not be fast enough to keep up")
	}

	return b, nil
}

func (m *Miner) computeVRF(ctx context.Context, addr address.Address, input []byte) ([]byte, error) {
	w, err := m.getMinerWorker(ctx, addr, nil)
	if err != nil {
		return nil, err
	}

	return gen.ComputeVRF(ctx, m.api.WalletSign, w, addr, gen.DSepTicket, input)
}

func (m *Miner) getMinerWorker(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	ret, err := m.api.StateCall(ctx, &types.Message{
		From:   addr,
		To:     addr,
		Method: actors.MAMethods.GetWorkerAddr,
	}, ts)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to get miner worker addr: %w", err)
	}

	if ret.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("failed to get miner worker addr (exit code %d)", ret.ExitCode)
	}

	w, err := address.NewFromBytes(ret.Return)
	if err != nil {
		return address.Undef, xerrors.Errorf("GetWorkerAddr returned malformed address: %w", err)
	}

	return w, nil
}

func (m *Miner) computeTicket(ctx context.Context, addr address.Address, base *MiningBase) (*types.Ticket, error) {

	vrfBase := base.ts.MinTicket().VRFProof

	vrfOut, err := m.computeVRF(ctx, addr, vrfBase)
	if err != nil {
		return nil, err
	}

	return &types.Ticket{
		VRFProof: vrfOut,
	}, nil
}

func (m *Miner) actorLookup(ctx context.Context, addr address.Address, ts *types.TipSet) (uint64, *types.BigInt, error) {
	// TODO: strong opportunities for some caching in this method
	act, err := m.api.StateGetActor(ctx, addr, ts)
	if err != nil {
		return 0, nil, xerrors.Errorf("looking up actor failed: %w", err)
	}

	msgs, err := m.api.ChainGetTipSetMessages(ctx, ts.Key())
	if err != nil {
		return 0, nil, xerrors.Errorf("failed to get tipset messages: %w", err)
	}

	curnonce := act.Nonce

	sort.Slice(msgs, func(i, j int) bool { // TODO: is this actually needed?
		return msgs[i].Message.Nonce < msgs[j].Message.Nonce
	})

	max := int64(-2)

	for _, m := range msgs {
		if m.Message.From == addr {
			max = int64(m.Message.Nonce)
		}
	}

	max++ // next unapplied nonce

	if max != -1 && uint64(max) != curnonce {
		return 0, nil, xerrors.Errorf("tipset messages from %s have too low nonce %d, expected %d, h: %d", addr, max, curnonce, ts.Height())
	}

	return curnonce, &act.Balance, nil
}

func (m *Miner) createBlock(base *MiningBase, addr address.Address, ticket *types.Ticket, proof *types.EPostProof, pending []*types.SignedMessage) (*types.BlockMsg, error) {
	msgs, err := selectMessages(context.TODO(), m.actorLookup, base, pending)
	if err != nil {
		return nil, xerrors.Errorf("message filtering failed: %w", err)
	}

	uts := base.ts.MinTimestamp() + uint64(build.BlockDelay*(base.nullRounds+1))

	nheight := base.ts.Height() + base.nullRounds + 1

	// why even return this? that api call could just submit it for us
	return m.api.MinerCreateBlock(context.TODO(), addr, base.ts, ticket, proof, msgs, nheight, uint64(uts))
}

type actorLookup func(context.Context, address.Address, *types.TipSet) (uint64, *types.BigInt, error)

func countFrom(msgs []*types.SignedMessage, from address.Address) (out int) {
	for _, msg := range msgs {
		if msg.Message.From == from {
			out++
		}
	}
	return out
}

func selectMessages(ctx context.Context, al actorLookup, base *MiningBase, msgs []*types.SignedMessage) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, 0, len(msgs))
	inclNonces := make(map[address.Address]uint64)
	inclBalances := make(map[address.Address]types.BigInt)

	sort.Slice(msgs, func(i, j int) bool { // TODO: is this actually needed?
		return msgs[i].Message.Nonce < msgs[j].Message.Nonce
	})

	for _, msg := range msgs {
		if msg.Message.To == address.Undef {
			log.Warnf("message in mempool had bad 'To' address")
			continue
		}

		from := msg.Message.From

		if _, ok := inclNonces[from]; !ok {
			nonce, balance, err := al(ctx, from, base.ts)
			if err != nil {
				return nil, xerrors.Errorf("failed to check message sender balance: %w", err)
			}

			inclNonces[from] = nonce
			inclBalances[from] = *balance
		}

		if inclBalances[from].LessThan(msg.Message.RequiredFunds()) {
			log.Warnf("message in mempool does not have enough funds: %s", msg.Cid())
			continue
		}

		if msg.Message.Nonce > inclNonces[from] {
			log.Warnf("message in mempool has too high of a nonce (%d > %d) %s (%d pending for orig)", msg.Message.Nonce, inclNonces[from], msg.Cid(), countFrom(msgs, from))
			continue
		}

		if msg.Message.Nonce < inclNonces[from] {
			log.Warnf("message in mempool has already used nonce (%d < %d), from %s, to %s, %s (%d pending for)", msg.Message.Nonce, inclNonces[from], msg.Message.From, msg.Message.To, msg.Cid(), countFrom(msgs, from))
			continue
		}

		inclNonces[from] = msg.Message.Nonce + 1
		inclBalances[from] = types.BigSub(inclBalances[from], msg.Message.RequiredFunds())

		out = append(out, msg)
	}
	return out, nil
}
