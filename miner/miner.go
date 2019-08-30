package miner

import (
	"context"
	"crypto/sha256"
	"math/big"
	"sync"
	"time"

	"github.com/filecoin-project/go-lotus/build"
	chain "github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/vdf"
	"github.com/filecoin-project/go-lotus/node/impl/full"

	logging "github.com/ipfs/go-log"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

var log = logging.Logger("miner")

type api struct {
	fx.In

	full.ChainAPI
	full.MpoolAPI
	full.WalletAPI
	full.StateAPI
}

func NewMiner(api api) *Miner {
	return &Miner{
		api:   api,
		Delay: build.BlockDelay,
	}
}

type Miner struct {
	api api

	lk        sync.Mutex
	addresses []address.Address
	stop      chan struct{}
	stopping  chan struct{}

	// time between blocks, network parameter
	Delay time.Duration

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

		base, err := m.GetBestMiningCandidate()
		if err != nil {
			log.Errorf("failed to get best mining candidate: %s", err)
			continue
		}

		b, err := m.mineOne(ctx, base)
		if err != nil {
			log.Errorf("mining block failed: %s", err)
			log.Warn("waiting 400ms before attempting to mine a block")
			time.Sleep(400 * time.Millisecond)
			continue
		}

		if b != nil {
			if err := m.api.ChainSubmitBlock(ctx, b); err != nil {
				log.Errorf("failed to submit newly mined block: %s", err)
			}
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

func (m *Miner) mineOne(ctx context.Context, base *MiningBase) (*chain.BlockMsg, error) {
	log.Info("attempting to mine a block on:", base.ts.Cids())
	ticket, err := m.scratchTicket(ctx, base)
	if err != nil {
		return nil, errors.Wrap(err, "scratching ticket failed")
	}

	win, proof, err := m.isWinnerNextRound(ctx, base)
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
	ret, err := m.api.ChainCall(ctx, &types.Message{
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

func (m *Miner) isWinnerNextRound(ctx context.Context, base *MiningBase) (bool, types.ElectionProof, error) {
	r, err := m.api.ChainGetRandomness(context.TODO(), base.ts)
	if err != nil {
		return false, nil, err
	}

	vrfout, err := m.computeVRF(ctx, r)
	if err != nil {
		return false, nil, xerrors.Errorf("failed to compute VRF: %w", err)
	}

	pow, err := m.api.StateMinerPower(ctx, m.addresses[0], base.ts)
	if err != nil {
		return false, nil, xerrors.Errorf("failed to check power: %w", err)
	}

	return powerCmp(vrfout, pow.MinerPower, pow.TotalPower), vrfout, nil
}

func powerCmp(vrfout []byte, mpow, totpow types.BigInt) bool {

	/*
		Need to check that
		h(vrfout) / 2^256 < minerPower / totalPower
	*/

	h := sha256.Sum256(vrfout)

	// 2^256
	rden := types.BigInt{big.NewInt(0).Exp(big.NewInt(2), big.NewInt(256), nil)}

	top := types.BigMul(rden, mpow)
	out := types.BigDiv(top, totpow)

	return types.BigCmp(types.BigFromBytes(h[:]), out) < 0
}

func (m *Miner) runVDF(ctx context.Context, input []byte) ([]byte, []byte, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-time.After(m.Delay):
	}

	return vdf.Run(input)
}

func (m *Miner) scratchTicket(ctx context.Context, base *MiningBase) (*types.Ticket, error) {
	var lastTicket *types.Ticket
	if len(base.tickets) > 0 {
		lastTicket = base.tickets[len(base.tickets)-1]
	} else {
		lastTicket = base.ts.MinTicket()
	}

	vrfOut, err := m.computeVRF(ctx, lastTicket.VDFResult)
	if err != nil {
		return nil, err
	}

	res, proof, err := m.runVDF(ctx, vrfOut)
	if err != nil {
		return nil, err
	}

	return &types.Ticket{
		VRFProof:  vrfOut,
		VDFResult: res,
		VDFProof:  proof,
	}, nil
}

func (m *Miner) createBlock(base *MiningBase, ticket *types.Ticket, proof types.ElectionProof) (*chain.BlockMsg, error) {

	pending, err := m.api.MpoolPending(context.TODO(), base.ts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pending messages")
	}

	msgs := m.selectMessages(pending)

	// why even return this? that api call could just submit it for us
	return m.api.MinerCreateBlock(context.TODO(), m.addresses[0], base.ts, append(base.tickets, ticket), proof, msgs, 0)
}

func (m *Miner) selectMessages(msgs []*types.SignedMessage) []*types.SignedMessage {
	// TODO: filter and select 'best' message if too many to fit in one block
	return msgs
}
