package miner

import (
	"context"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"

	logging "github.com/ipfs/go-log"
	"go.opencensus.io/trace"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

var log = logging.Logger("miner")

type waitFunc func(ctx context.Context) error

type api struct {
	fx.In

	full.ChainAPI
	full.SyncAPI
	full.MpoolAPI
	full.WalletAPI
	full.StateAPI
}

func NewMiner(api api) *Miner {
	return &Miner{
		api: api,
		waitFunc: func(ctx context.Context) error {
			// Wait around for half the block time in case other parents come in
			time.Sleep(build.BlockDelay * time.Second / 2)
			return nil
		},
	}
}

type Miner struct {
	api api

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
		if base.ts.Equals(lastBase.ts) && len(lastBase.tickets) == len(base.tickets) {
			log.Errorf("BestMiningCandidate from the previous round: %s (tkts:%d)", lastBase.ts.Cids(), len(lastBase.tickets))
			time.Sleep(build.BlockDelay * time.Second)
			continue
		}
		lastBase = *base

		blks := make([]*types.BlockMsg, 0)

		for _, addr := range addrs {
			b, err := m.mineOne(ctx, addr, base)
			if err != nil {
				log.Errorf("mining block failed: %s", err)
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
			nextRound := time.Unix(int64(base.ts.MinTimestamp()+uint64(build.BlockDelay*len(base.tickets))), 0)
			time.Sleep(time.Until(nextRound))
		}
	}
}

type MiningBase struct {
	ts      *types.TipSet
	tickets []*types.Ticket
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

	return &MiningBase{
		ts: bts,
	}, nil
}

func (m *Miner) mineOne(ctx context.Context, addr address.Address, base *MiningBase) (*types.BlockMsg, error) {
	log.Debugw("attempting to mine a block", "tipset", types.LogCids(base.ts.Cids()))
	ticket, err := m.scratchTicket(ctx, addr, base)
	if err != nil {
		return nil, xerrors.Errorf("scratching ticket failed: %w", err)
	}

	win, proof, err := gen.IsRoundWinner(ctx, base.ts, append(base.tickets, ticket), addr, &m.api)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if we win next round: %w", err)
	}

	if !win {
		m.submitNullTicket(base, ticket)
		return nil, nil
	}

	b, err := m.createBlock(base, addr, ticket, proof)
	if err != nil {
		return nil, xerrors.Errorf("failed to create block: %w", err)
	}
	log.Infow("mined new block", "cid", b.Cid())

	return b, nil
}

func (m *Miner) submitNullTicket(base *MiningBase, ticket *types.Ticket) {
	base.tickets = append(base.tickets, ticket)
	m.lastWork = base
}

func (m *Miner) computeVRF(ctx context.Context, addr address.Address, input []byte) ([]byte, error) {
	w, err := m.getMinerWorker(ctx, addr, nil)
	if err != nil {
		return nil, err
	}

	return gen.ComputeVRF(ctx, m.api.WalletSign, w, input)
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

func (m *Miner) scratchTicket(ctx context.Context, addr address.Address, base *MiningBase) (*types.Ticket, error) {
	var lastTicket *types.Ticket
	if len(base.tickets) > 0 {
		lastTicket = base.tickets[len(base.tickets)-1]
	} else {
		lastTicket = base.ts.MinTicket()
	}

	vrfOut, err := m.computeVRF(ctx, addr, lastTicket.VRFProof)
	if err != nil {
		return nil, err
	}

	return &types.Ticket{
		VRFProof: vrfOut,
	}, nil
}

func (m *Miner) createBlock(base *MiningBase, addr address.Address, ticket *types.Ticket, proof types.ElectionProof) (*types.BlockMsg, error) {

	pending, err := m.api.MpoolPending(context.TODO(), base.ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get pending messages: %w", err)
	}

	msgs, err := selectMessages(context.TODO(), m.api.StateGetActor, base, pending)
	if err != nil {
		return nil, xerrors.Errorf("message filtering failed: %w", err)
	}

	uts := base.ts.MinTimestamp() + uint64(build.BlockDelay*(len(base.tickets)+1))

	// why even return this? that api call could just submit it for us
	return m.api.MinerCreateBlock(context.TODO(), addr, base.ts, append(base.tickets, ticket), proof, msgs, uint64(uts))
}

type actorLookup func(context.Context, address.Address, *types.TipSet) (*types.Actor, error)

func selectMessages(ctx context.Context, al actorLookup, base *MiningBase, msgs []*types.SignedMessage) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, 0, len(msgs))
	inclNonces := make(map[address.Address]uint64)
	inclBalances := make(map[address.Address]types.BigInt)
	for _, msg := range msgs {
		if msg.Message.To == address.Undef {
			log.Warnf("message in mempool had bad 'To' address")
			continue
		}

		from := msg.Message.From
		act, err := al(ctx, from, base.ts)
		if err != nil {
			return nil, xerrors.Errorf("failed to check message sender balance: %w", err)
		}

		if _, ok := inclNonces[from]; !ok {
			inclNonces[from] = act.Nonce
			inclBalances[from] = act.Balance
		}

		if inclBalances[from].LessThan(msg.Message.RequiredFunds()) {
			log.Warnf("message in mempool does not have enough funds: %s", msg.Cid())
			continue
		}

		if msg.Message.Nonce > inclNonces[from] {
			log.Warnf("message in mempool has too high of a nonce (%d > %d) %s", msg.Message.Nonce, inclNonces[from], msg.Cid())
			continue
		}

		if msg.Message.Nonce < inclNonces[from] {
			log.Warnf("message in mempool has already used nonce (%d < %d) %s", msg.Message.Nonce, inclNonces[from], msg.Cid())
			continue
		}

		inclNonces[from] = msg.Message.Nonce + 1
		inclBalances[from] = types.BigSub(inclBalances[from], msg.Message.RequiredFunds())

		out = append(out, msg)
	}
	return out, nil
}
