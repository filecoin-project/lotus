package miner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	logging "github.com/ipfs/go-log"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	chain "github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/vdf"
)

var log = logging.Logger("miner")

type api interface {
	ChainCall(context.Context, *types.Message, *types.TipSet) (*types.MessageReceipt, error)

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
	MinerCreateBlock(context.Context, address.Address, *types.TipSet, []*types.Ticket, types.ElectionProof, []*types.SignedMessage) (*chain.BlockMsg, error)

	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
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
	ctx, span := trace.StartSpan(ctx, "/mine")
	defer span.End()
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
	w, err := m.getMinerWorker(ctx, m.address, nil)
	if err != nil {
		return nil, err
	}

	sig, err := m.api.WalletSign(ctx, w, input)
	if err != nil {
		return nil, err
	}

	if sig.Type != types.KTBLS {
		return nil, fmt.Errorf("miner worker address was not a BLS key")
	}

	return sig.Data, nil
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

	mpow, totpow, err := m.getPowerForTipset(ctx, m.address, base.ts)
	if err != nil {
		return false, nil, xerrors.Errorf("failed to check power: %w", err)
	}

	return powerCmp(vrfout, mpow, totpow), vrfout, nil
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

func (m *Miner) getPowerForTipset(ctx context.Context, maddr address.Address, ts *types.TipSet) (types.BigInt, types.BigInt, error) {
	var err error
	enc, err := actors.SerializeParams(&actors.PowerLookupParams{maddr})
	if err != nil {
		return types.EmptyInt, types.EmptyInt, err
	}

	ret, err := m.api.ChainCall(ctx, &types.Message{
		From:   maddr,
		To:     actors.StorageMarketAddress,
		Method: actors.SMAMethods.PowerLookup,
		Params: enc,
	}, ts)
	if err != nil {
		return types.EmptyInt, types.EmptyInt, xerrors.Errorf("failed to get miner power from chain: %w", err)
	}
	if ret.ExitCode != 0 {
		return types.EmptyInt, types.EmptyInt, xerrors.Errorf("failed to get miner power from chain (exit code %d)", ret.ExitCode)
	}

	mpow := types.BigFromBytes(ret.Return)

	ret, err = m.api.ChainCall(ctx, &types.Message{
		From:   maddr,
		To:     actors.StorageMarketAddress,
		Method: actors.SMAMethods.GetTotalStorage,
	}, ts)
	if err != nil {
		return types.EmptyInt, types.EmptyInt, xerrors.Errorf("failed to get total power from chain: %w", err)
	}
	if ret.ExitCode != 0 {
		return types.EmptyInt, types.EmptyInt, xerrors.Errorf("failed to get total power from chain (exit code %d)", ret.ExitCode)
	}

	tpow := types.BigFromBytes(ret.Return)

	return mpow, tpow, nil
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
	return m.api.MinerCreateBlock(context.TODO(), m.address, base.ts, append(base.tickets, ticket), proof, msgs)
}

func (m *Miner) selectMessages(msgs []*types.SignedMessage) []*types.SignedMessage {
	// TODO: filter and select 'best' message if too many to fit in one block
	return msgs
}
