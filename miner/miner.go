package miner

import (
	"context"
	"sync"
	"time"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/impl/full"

	logging "github.com/ipfs/go-log"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

var log = logging.Logger("miner")

type waitFunc func(ctx context.Context) error

type api struct {
	fx.In

	full.ChainAPI
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
		if len(m.addresses) > 1 || m.addresses[0] != addr {
			return errors.New("mining with more than one storage miner instance not supported yet") // TODO !
		}

		log.Warnf("miner.Register called more than once for actor '%s'", addr)
		return xerrors.Errorf("miner.Register called more than once for actor '%s'", addr)
	}

	m.addresses = append(m.addresses, addr)
	m.stop = make(chan struct{})

	go m.mine(context.TODO())

	return nil
}

func (m *Miner) Unregister(ctx context.Context, addr address.Address) error {
	m.lk.Lock()
	if len(m.addresses) == 0 {
		m.lk.Unlock()
		return xerrors.New("no addresses registered")
	}

	if len(m.addresses) > 1 {
		m.lk.Unlock()
		log.Errorf("UNREGISTER NOT IMPLEMENTED FOR MORE THAN ONE ADDRESS!")
		return xerrors.New("can't unregister when more than one actor is registered: not implemented")
	}

	if m.addresses[0] != addr {
		m.lk.Unlock()
		return xerrors.New("unregister: address not found")
	}

	// Unregistering last address, stop mining first
	if m.stop != nil {
		if m.stopping == nil {
			m.stopping = make(chan struct{})
			close(m.stop)
		}
		stopping := m.stopping
		m.lk.Unlock()
		select {
		case <-stopping:
		case <-ctx.Done():
			return ctx.Err()
		}
		m.lk.Lock()
	}

	m.addresses = []address.Address{}

	m.lk.Unlock()
	return nil
}

func (m *Miner) mine(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "/mine")
	defer span.End()

	var lastBase *types.TipSet

	for {
		select {
		case <-m.stop:
			m.lk.Lock()

			close(m.stopping)
			m.stop = nil
			m.stopping = nil

			m.lk.Unlock()

			return
		default:
		}

		// Sleep a small amount in order to wait for other blocks to arrive
		if err := m.waitFunc(ctx); err != nil {
			log.Error(err)
			return
		}

		base, err := m.GetBestMiningCandidate()
		if err != nil {
			log.Errorf("failed to get best mining candidate: %s", err)
			continue
		}
		if base.ts.Equals(lastBase) {
			log.Error("BestMiningCandidate from the previous round: %s", lastBase.Cids())
			time.Sleep(build.BlockDelay)
			continue
		}
		lastBase = base.ts

		b, err := m.mineOne(ctx, base)
		if err != nil {
			log.Errorf("mining block failed: %s", err)
			log.Warn("waiting 400ms before attempting to mine a block")
			time.Sleep(400 * time.Millisecond)
			continue
		}

		if b != nil {
			btime := time.Unix(int64(b.Header.Timestamp), 0)
			if time.Now().Before(btime) {
				time.Sleep(time.Until(btime))
			} else {
				log.Warnf("Mined block in the past: b.T: %s, T: %s, dT: %s", btime, time.Now(), time.Now().Sub(btime))
			}

			if err := m.api.ChainSubmitBlock(ctx, b); err != nil {
				log.Errorf("failed to submit newly mined block: %s", err)
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

func (m *Miner) GetBestMiningCandidate() (*MiningBase, error) {
	bts, err := m.api.ChainHead(context.TODO())
	if err != nil {
		return nil, err
	}

	if m.lastWork != nil {
		if m.lastWork.ts.Equals(bts) {
			return m.lastWork, nil
		}

		if types.BigCmp(bts.Weight(), m.lastWork.ts.Weight()) <= 0 {
			return m.lastWork, nil
		}
	}

	return &MiningBase{
		ts: bts,
	}, nil
}

func (m *Miner) mineOne(ctx context.Context, base *MiningBase) (*types.BlockMsg, error) {
	log.Debug("attempting to mine a block on:", base.ts.Cids())
	ticket, err := m.scratchTicket(ctx, base)
	if err != nil {
		return nil, errors.Wrap(err, "scratching ticket failed")
	}

	win, proof, err := gen.IsRoundWinner(ctx, base.ts, append(base.tickets, ticket), m.addresses[0], &m.api)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check if we win next round")
	}

	if !win {
		m.submitNullTicket(base, ticket)
		return nil, nil
	}

	b, err := m.createBlock(base, ticket, proof)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create block")
	}
	log.Infof("mined new block: %s", b.Cid())

	return b, nil
}

func (m *Miner) submitNullTicket(base *MiningBase, ticket *types.Ticket) {
	base.tickets = append(base.tickets, ticket)
	m.lastWork = base
}

func (m *Miner) computeVRF(ctx context.Context, input []byte) ([]byte, error) {
	w, err := m.getMinerWorker(ctx, m.addresses[0], nil)
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

func (m *Miner) scratchTicket(ctx context.Context, base *MiningBase) (*types.Ticket, error) {
	var lastTicket *types.Ticket
	if len(base.tickets) > 0 {
		lastTicket = base.tickets[len(base.tickets)-1]
	} else {
		lastTicket = base.ts.MinTicket()
	}

	vrfOut, err := m.computeVRF(ctx, lastTicket.VRFProof)
	if err != nil {
		return nil, err
	}

	return &types.Ticket{
		VRFProof: vrfOut,
	}, nil
}

func (m *Miner) createBlock(base *MiningBase, ticket *types.Ticket, proof types.ElectionProof) (*types.BlockMsg, error) {

	pending, err := m.api.MpoolPending(context.TODO(), base.ts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pending messages")
	}

	msgs, err := selectMessages(context.TODO(), m.api.StateGetActor, base, pending)
	if err != nil {
		return nil, xerrors.Errorf("message filtering failed: %w", err)
	}

	uts := base.ts.MinTimestamp() + uint64(build.BlockDelay*(len(base.tickets)+1))

	// why even return this? that api call could just submit it for us
	return m.api.MinerCreateBlock(context.TODO(), m.addresses[0], base.ts, append(base.tickets, ticket), proof, msgs, uint64(uts))
}

type actorLookup func(context.Context, address.Address, *types.TipSet) (*types.Actor, error)

func selectMessages(ctx context.Context, al actorLookup, base *MiningBase, msgs []*types.SignedMessage) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, 0, len(msgs))
	inclNonces := make(map[address.Address]uint64)
	inclBalances := make(map[address.Address]types.BigInt)
	for _, msg := range msgs {
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
