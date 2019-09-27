package gen

import (
	"context"

	amt "github.com/filecoin-project/go-amt-ipld"
	bls "github.com/filecoin-project/go-bls-sigs"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/pkg/errors"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/stmgr"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"github.com/filecoin-project/go-lotus/chain/wallet"
)

func MinerCreateBlock(ctx context.Context, sm *stmgr.StateManager, w *wallet.Wallet, miner address.Address, parents *types.TipSet, tickets []*types.Ticket, proof types.ElectionProof, msgs []*types.SignedMessage, timestamp uint64) (*types.FullBlock, error) {
	st, recpts, err := sm.TipSetState(parents.Cids())
	if err != nil {
		return nil, errors.Wrap(err, "failed to load tipset state")
	}

	height := parents.Height() + uint64(len(tickets))

	r := vm.NewChainRand(sm.ChainStore(), parents.Cids(), parents.Height(), tickets)
	vmi, err := vm.NewVM(st, height, r, miner, sm.ChainStore())
	if err != nil {
		return nil, err
	}

	owner, err := stmgr.GetMinerOwner(ctx, sm, st, miner)
	if err != nil {
		return nil, xerrors.Errorf("failed to get miner owner: %w", err)
	}

	worker, err := stmgr.GetMinerWorker(ctx, sm, st, miner)
	if err != nil {
		return nil, xerrors.Errorf("failed to get miner worker: %w", err)
	}
	networkBalance, err := vmi.ActorBalance(actors.NetworkAddress)
	if err != nil {
		return nil, xerrors.Errorf("failed to get network balance: %w", err)
	}

	// apply miner reward
	if err := vmi.TransferFunds(actors.NetworkAddress, owner, vm.MiningReward(networkBalance)); err != nil {
		return nil, err
	}

	next := &types.BlockHeader{
		Miner:                 miner,
		Parents:               parents.Cids(),
		Tickets:               tickets,
		Height:                height,
		Timestamp:             timestamp,
		ElectionProof:         proof,
		ParentStateRoot:       st,
		ParentMessageReceipts: recpts,
	}

	var blsMessages []*types.Message
	var secpkMessages []*types.SignedMessage

	var blsMsgCids, secpkMsgCids []cid.Cid
	var blsSigs []types.Signature
	for _, msg := range msgs {
		if msg.Signature.TypeCode() == types.IKTBLS {
			blsSigs = append(blsSigs, msg.Signature)
			blsMessages = append(blsMessages, &msg.Message)

			c, err := sm.ChainStore().PutMessage(&msg.Message)
			if err != nil {
				return nil, err
			}

			blsMsgCids = append(blsMsgCids, c)
		} else {
			secpkMsgCids = append(secpkMsgCids, msg.Cid())
			secpkMessages = append(secpkMessages, msg)
		}
	}

	bs := amt.WrapBlockstore(sm.ChainStore().Blockstore())
	blsmsgroot, err := amt.FromArray(bs, toIfArr(blsMsgCids))
	if err != nil {
		return nil, xerrors.Errorf("building bls amt: %w", err)
	}
	secpkmsgroot, err := amt.FromArray(bs, toIfArr(secpkMsgCids))
	if err != nil {
		return nil, xerrors.Errorf("building secpk amt: %w", err)
	}

	mmcid, err := bs.Put(&types.MsgMeta{
		BlsMessages:   blsmsgroot,
		SecpkMessages: secpkmsgroot,
	})
	if err != nil {
		return nil, err
	}
	next.Messages = mmcid

	aggSig, err := aggregateSignatures(blsSigs)
	if err != nil {
		return nil, err
	}

	next.BLSAggregate = aggSig
	pweight := sm.ChainStore().Weight(parents)
	next.ParentWeight = types.NewInt(pweight)

	// TODO: set timestamp

	nosigbytes, err := next.SigningBytes()
	if err != nil {
		return nil, xerrors.Errorf("failed to get signing bytes for block: %w", err)
	}

	cst := hamt.CSTFromBstore(sm.ChainStore().Blockstore())
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

func toIfArr(cids []cid.Cid) []cbg.CBORMarshaler {
	out := make([]cbg.CBORMarshaler, 0, len(cids))
	for _, c := range cids {
		oc := cbg.CborCid(c)
		out = append(out, &oc)
	}
	return out
}
