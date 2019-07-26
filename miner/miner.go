package miner

import (
	"context"
	"time"

	logging "github.com/ipfs/go-log"
	"github.com/pkg/errors"

	chain "github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

var log = logging.Logger("miner")

type api interface {
	ChainSubmitBlock(context.Context, *chain.BlockMsg) error

	// returns a set of messages that havent been included in the chain as of
	// the given tipset
	MpoolPending(ctx context.Context, base *types.TipSet) ([]*types.SignedMessage, error)

	// Returns the best tipset for the miner to mine on top of.
	// TODO: Not sure this feels right (including the messages api). Miners
	// will likely want to have more control over exactly which blocks get
	// mined on, and which messages are included.
	ChainHead(context.Context) (*types.TipSet, error)

	// returns the lookback randomness from the chain used for the election
	ChainGetRandomness(context.Context, *types.TipSet) ([]byte, error)

	// create a block
	// it seems realllllly annoying to do all the actions necessary to build a
	// block through the API. so, we just add the block creation to the API
	// now, all the 'miner' does is check if they win, and call create block
	MinerCreateBlock(context.Context, address.Address, *types.TipSet, []types.Ticket, types.ElectionProof, []*types.SignedMessage) (*chain.BlockMsg, error)
}

func NewMiner(api api, addr address.Address) *Miner {
	return &Miner{
		api:     api,
		address: addr,
		Delay:   time.Second * 4,
	}
}

type Miner struct {
	api api

	address address.Address

	// time between blocks, network parameter
	Delay time.Duration

	lastWork *MiningBase
}

func (m *Miner) Mine(ctx context.Context) {
	for {
		base, err := m.GetBestMiningCandidate()
		if err != nil {
			log.Errorf("failed to get best mining candidate: %s", err)
			continue
		}

		b, err := m.mineOne(ctx, base)
		if err != nil {
			log.Errorf("mining block failed: %s", err)
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
	tickets []types.Ticket
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

		if bts.Weight() <= m.lastWork.ts.Weight() {
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

	win, proof, err := m.isWinnerNextRound(base)
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

func (m *Miner) submitNullTicket(base *MiningBase, ticket types.Ticket) {
	base.tickets = append(base.tickets, ticket)
	m.lastWork = base
}

func (m *Miner) isWinnerNextRound(base *MiningBase) (bool, types.ElectionProof, error) {
	r, err := m.api.ChainGetRandomness(context.TODO(), base.ts)
	if err != nil {
		return false, nil, err
	}

	_ = r // TODO: use this to properly compute the election proof

	return true, []byte("election prooooof"), nil
}

func (m *Miner) scratchTicket(ctx context.Context, base *MiningBase) (types.Ticket, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(m.Delay):
	}

	return []byte("this is a ticket"), nil
}

func (m *Miner) createBlock(base *MiningBase, ticket types.Ticket, proof types.ElectionProof) (*chain.BlockMsg, error) {

	pending, err := m.api.MpoolPending(context.TODO(), base.ts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pending messages")
	}

	msgs := m.selectMessages(pending)

	// why even return this? that api call could just submit it for us
	return m.api.MinerCreateBlock(context.TODO(), m.address, base.ts, append(base.tickets, ticket), proof, msgs)
}

func (m *Miner) selectMessages(msgs []*types.SignedMessage) []*types.SignedMessage {
	// TODO: filter and select 'best' message if too many to fit in one block
	return msgs
}
