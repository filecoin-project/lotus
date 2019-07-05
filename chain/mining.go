package chain

import (
	"context"
	"fmt"
	"time"

	bls "github.com/filecoin-project/go-filecoin/bls-signatures"
	"github.com/filecoin-project/go-lotus/chain/address"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/pkg/errors"
	sharray "github.com/whyrusleeping/sharray"
)

type Miner struct {
	cs         *ChainStore
	newBlockCB func(*FullBlock)

	maddr address.Address
	mpool *MessagePool

	Delay time.Duration

	candidate *MiningBase
}

func NewMiner(cs *ChainStore, maddr address.Address, mpool *MessagePool, newBlockCB func(*FullBlock)) *Miner {
	return &Miner{
		cs:         cs,
		newBlockCB: newBlockCB,
		maddr:      maddr,
		mpool:      mpool,
		Delay:      time.Second * 2,
	}
}

type MiningBase struct {
	ts      *TipSet
	tickets []Ticket
}

func (m *Miner) Mine(ctx context.Context) {
	log.Error("mining...")
	defer log.Error("left mining...")
	for {
		base := m.GetBestMiningCandidate()

		b, err := m.mineOne(ctx, base)
		if err != nil {
			log.Error(err)
			continue
		}

		if b != nil {
			m.submitNewBlock(b)
		}
	}
}

func (m *Miner) GetBestMiningCandidate() *MiningBase {
	best := m.cs.GetHeaviestTipSet()
	if m.candidate == nil {
		if best == nil {
			panic("no best candidate!")
		}
		return &MiningBase{
			ts: best,
		}
	}

	if m.cs.Weight(best) > m.cs.Weight(m.candidate.ts) {
		return &MiningBase{
			ts: best,
		}
	}

	return m.candidate
}

func (m *Miner) submitNullTicket(base *MiningBase, ticket Ticket) {
	panic("nyi")
}

func (m *Miner) isWinnerNextRound(base *MiningBase) (bool, ElectionProof, error) {
	return true, []byte("election prooooof"), nil
}

func (m *Miner) scratchTicket(ctx context.Context, base *MiningBase) (Ticket, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(m.Delay):
	}

	return []byte("this is a ticket"), nil
}

func (m *Miner) mineOne(ctx context.Context, base *MiningBase) (*FullBlock, error) {
	log.Info("mine one")
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
	fmt.Println("created new block:", b.Cid())

	return b, nil
}

func (m *Miner) submitNewBlock(b *FullBlock) {
	if err := m.cs.AddBlock(b.Header); err != nil {
		log.Error("failed to add new block to chainstore: ", err)
	}
	m.newBlockCB(b)
}

func miningRewardForBlock(base *TipSet) BigInt {
	return NewInt(10000)
}

func (m *Miner) createBlock(base *MiningBase, ticket Ticket, proof ElectionProof) (*FullBlock, error) {
	st, err := m.cs.TipSetState(base.ts.Cids())
	if err != nil {
		return nil, errors.Wrap(err, "failed to load tipset state")
	}

	height := base.ts.Height() + uint64(len(base.tickets)) + 1

	vm, err := NewVM(st, height, m.maddr, m.cs)

	// apply miner reward
	if err := vm.TransferFunds(NetworkAddress, m.maddr, miningRewardForBlock(base.ts)); err != nil {
		return nil, err
	}

	next := &BlockHeader{
		Miner:   m.maddr,
		Parents: base.ts.Cids(),
		Tickets: append(base.tickets, ticket),
		Height:  height,
	}

	pending := m.mpool.Pending()
	fmt.Printf("adding %d messages to block...", len(pending))
	var msgCids []cid.Cid
	var blsSigs []Signature
	var receipts []interface{}
	for _, msg := range pending {
		if msg.Signature.TypeCode() == 2 {
			blsSigs = append(blsSigs, msg.Signature)

			blk, err := msg.Message.ToStorageBlock()
			if err != nil {
				return nil, err
			}
			if err := m.cs.bs.Put(blk); err != nil {
				return nil, err
			}

			msgCids = append(msgCids, blk.Cid())
		} else {
			msgCids = append(msgCids, msg.Cid())
		}
		rec, err := vm.ApplyMessage(&msg.Message)
		if err != nil {
			return nil, errors.Wrap(err, "apply message failure")
		}

		receipts = append(receipts, rec)
	}

	cst := hamt.CSTFromBstore(m.cs.bs)
	msgroot, err := sharray.Build(context.TODO(), 4, toIfArr(msgCids), cst)
	if err != nil {
		return nil, err
	}
	next.Messages = msgroot

	rectroot, err := sharray.Build(context.TODO(), 4, receipts, cst)
	if err != nil {
		return nil, err
	}
	next.MessageReceipts = rectroot

	stateRoot, err := vm.Flush(context.TODO())
	if err != nil {
		return nil, errors.Wrap(err, "flushing state tree failed")
	}

	aggSig, err := aggregateSignatures(blsSigs)
	if err != nil {
		return nil, err
	}

	next.BLSAggregate = aggSig
	next.StateRoot = stateRoot
	pweight := m.cs.Weight(base.ts)
	next.ParentWeight = NewInt(pweight)

	fullBlock := &FullBlock{
		Header:   next,
		Messages: pending,
	}

	return fullBlock, nil
}

func aggregateSignatures(sigs []Signature) (Signature, error) {
	var blsSigs []bls.Signature
	for _, s := range sigs {
		var bsig bls.Signature
		copy(bsig[:], s.Data)
		blsSigs = append(blsSigs, bsig)
	}

	aggSig := bls.Aggregate(blsSigs)
	return Signature{
		Type: KTBLS,
		Data: aggSig[:],
	}, nil
}

func toIfArr(cids []cid.Cid) []interface{} {
	out := make([]interface{}, 0, len(cids))
	for _, c := range cids {
		out = append(out, c)
	}
	return out
}
