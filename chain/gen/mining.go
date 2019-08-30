package gen

import (
	"context"

	bls "github.com/filecoin-project/go-bls-sigs"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/pkg/errors"
	sharray "github.com/whyrusleeping/sharray"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"github.com/filecoin-project/go-lotus/chain/wallet"
)

func MinerCreateBlock(ctx context.Context, cs *store.ChainStore, w *wallet.Wallet, miner address.Address, parents *types.TipSet, tickets []*types.Ticket, proof types.ElectionProof, msgs []*types.SignedMessage, timestamp uint64) (*types.FullBlock, error) {
	st, err := cs.TipSetState(parents.Cids())
	if err != nil {
		return nil, errors.Wrap(err, "failed to load tipset state")
	}

	height := parents.Height() + uint64(len(tickets))

	vmi, err := vm.NewVM(st, height, miner, cs)
	if err != nil {
		return nil, err
	}

	owner, err := getMinerOwner(ctx, cs, st, miner)
	if err != nil {
		return nil, xerrors.Errorf("failed to get miner owner: %w", err)
	}

	worker, err := getMinerWorker(ctx, cs, st, miner)
	if err != nil {
		return nil, xerrors.Errorf("failed to get miner worker: %w", err)
	}

	// apply miner reward
	if err := vmi.TransferFunds(actors.NetworkAddress, owner, vm.MiningRewardForBlock(parents)); err != nil {
		return nil, err
	}

	next := &types.BlockHeader{
		Miner:     miner,
		Parents:   parents.Cids(),
		Tickets:   tickets,
		Height:    height,
		Timestamp: timestamp,
	}

	var blsMessages []*types.Message
	var secpkMessages []*types.SignedMessage

	var blsMsgCids, secpkMsgCids []cid.Cid
	var blsSigs []types.Signature
	for _, msg := range msgs {
		if msg.Signature.TypeCode() == types.IKTBLS {
			blsSigs = append(blsSigs, msg.Signature)
			blsMessages = append(blsMessages, &msg.Message)

			c, err := cs.PutMessage(&msg.Message)
			if err != nil {
				return nil, err
			}

			blsMsgCids = append(blsMsgCids, c)
		} else {
			secpkMsgCids = append(secpkMsgCids, msg.Cid())
			secpkMessages = append(secpkMessages, msg)
		}
	}

	var receipts []interface{}
	for _, msg := range blsMessages {
		rec, err := vmi.ApplyMessage(ctx, msg)
		if err != nil {
			return nil, errors.Wrap(err, "apply message failure")
		}

		receipts = append(receipts, rec.MessageReceipt)
	}
	for _, msg := range secpkMessages {
		rec, err := vmi.ApplyMessage(ctx, &msg.Message)
		if err != nil {
			return nil, errors.Wrap(err, "apply message failure")
		}

		receipts = append(receipts, rec.MessageReceipt)
	}

	cst := hamt.CSTFromBstore(cs.Blockstore())
	blsmsgroot, err := sharray.Build(context.TODO(), 4, toIfArr(blsMsgCids), cst)
	if err != nil {
		return nil, err
	}
	secpkmsgroot, err := sharray.Build(context.TODO(), 4, toIfArr(secpkMsgCids), cst)
	if err != nil {
		return nil, err
	}

	mmcid, err := cst.Put(context.TODO(), &types.MsgMeta{
		BlsMessages:   blsmsgroot,
		SecpkMessages: secpkmsgroot,
	})
	if err != nil {
		return nil, err
	}
	next.Messages = mmcid

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

	// TODO: set timestamp

	nosigbytes, err := next.Serialize()
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize block header with no signature: %w", err)
	}

	waddr, err := vm.ResolveToKeyAddr(vmi.StateTree(), cst, worker)
	if err != nil {
		return nil, xerrors.Errorf("failed to resolve miner address to key address: %w", err)
	}

	sig, err := w.Sign(ctx, waddr, nosigbytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign new block: %w", err)
	}

	next.BlockSig = *sig

	fullBlock := &types.FullBlock{
		Header:        next,
		BlsMessages:   blsMessages,
		SecpkMessages: secpkMessages,
	}

	return fullBlock, nil
}

func getMinerWorker(ctx context.Context, cs *store.ChainStore, state cid.Cid, maddr address.Address) (address.Address, error) {
	rec, err := vm.CallRaw(ctx, cs, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetWorkerAddr,
	}, state, 0)
	if err != nil {
		return address.Undef, err
	}

	if rec.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getWorker failed with exit code %d", rec.ExitCode)
	}

	return address.NewFromBytes(rec.Return)
}

func getMinerOwner(ctx context.Context, cs *store.ChainStore, state cid.Cid, maddr address.Address) (address.Address, error) {
	rec, err := vm.CallRaw(ctx, cs, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetOwner,
	}, state, 0)
	if err != nil {
		return address.Undef, err
	}

	if rec.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getOwner failed with exit code %d", rec.ExitCode)
	}

	return address.NewFromBytes(rec.Return)
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
