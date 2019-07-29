package gen

import (
	"context"

	bls "github.com/filecoin-project/go-bls-sigs"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/pkg/errors"
	sharray "github.com/whyrusleeping/sharray"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
)

func MinerCreateBlock(ctx context.Context, cs *store.ChainStore, miner address.Address, parents *types.TipSet, tickets []types.Ticket, proof types.ElectionProof, msgs []*types.SignedMessage) (*types.FullBlock, error) {
	st, err := cs.TipSetState(parents.Cids())
	if err != nil {
		return nil, errors.Wrap(err, "failed to load tipset state")
	}

	height := parents.Height() + uint64(len(tickets))

	vmi, err := vm.NewVM(st, height, miner, cs)
	if err != nil {
		return nil, err
	}

	// apply miner reward
	if err := vmi.TransferFunds(actors.NetworkAddress, miner, vm.MiningRewardForBlock(parents)); err != nil {
		return nil, err
	}

	next := &types.BlockHeader{
		Miner:   miner,
		Parents: parents.Cids(),
		Tickets: tickets,
		Height:  height,
	}

	var msgCids []cid.Cid
	var blsSigs []types.Signature
	var receipts []interface{}
	for _, msg := range msgs {
		if msg.Signature.TypeCode() == 2 {
			blsSigs = append(blsSigs, msg.Signature)

			c, err := cs.PutMessage(&msg.Message)
			if err != nil {
				return nil, err
			}

			msgCids = append(msgCids, c)
		} else {
			msgCids = append(msgCids, msg.Cid())
		}
		rec, err := vmi.ApplyMessage(ctx, &msg.Message)
		if err != nil {
			return nil, errors.Wrap(err, "apply message failure")
		}

		receipts = append(receipts, rec)
	}

	cst := hamt.CSTFromBstore(cs.Blockstore())
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

	stateRoot, err := vmi.Flush(context.TODO())
	if err != nil {
		return nil, errors.Wrap(err, "flushing state tree failed")
	}

	aggSig, err := aggregateSignatures(blsSigs)
	if err != nil {
		return nil, err
	}

	next.BLSAggregate = aggSig
	next.StateRoot = stateRoot
	pweight := cs.Weight(parents)
	next.ParentWeight = types.NewInt(pweight)

	fullBlock := &types.FullBlock{
		Header:   next,
		Messages: msgs,
	}

	return fullBlock, nil
}

func aggregateSignatures(sigs []types.Signature) (types.Signature, error) {
	var blsSigs []bls.Signature
	for _, s := range sigs {
		var bsig bls.Signature
		copy(bsig[:], s.Data)
		blsSigs = append(blsSigs, bsig)
	}

	aggSig := bls.Aggregate(blsSigs)
	return types.Signature{
		Type: types.KTBLS,
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
