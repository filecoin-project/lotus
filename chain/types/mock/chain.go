package mock

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/ipfs/go-cid"
)

func Address(i uint64) address.Address {
	a, err := address.NewIDAddress(i)
	if err != nil {
		panic(err)
	}
	return a
}

func MkMessage(from, to address.Address, nonce uint64, w *wallet.Wallet) *types.SignedMessage {
	msg := &types.Message{
		To:       to,
		From:     from,
		Value:    types.NewInt(1),
		Nonce:    nonce,
		GasLimit: types.NewInt(1),
		GasPrice: types.NewInt(0),
	}

	sig, err := w.Sign(context.TODO(), from, msg.Cid().Bytes())
	if err != nil {
		panic(err)
	}
	return &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}
}

func MkBlock(parents *types.TipSet, weightInc uint64, ticketNonce uint64) *types.BlockHeader {
	addr := Address(123561)

	c, err := cid.Decode("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")
	if err != nil {
		panic(err)
	}

	var pcids []cid.Cid
	var height uint64
	weight := types.NewInt(weightInc)
	if parents != nil {
		pcids = parents.Cids()
		height = parents.Height() + 1
		weight = types.BigAdd(parents.Blocks()[0].ParentWeight, weight)
	}

	return &types.BlockHeader{
		Miner: addr,
		EPostProof: types.EPostProof{
			Proof: []byte("election post proof proof"),
		},
		Ticket: &types.Ticket{
			VRFProof: []byte(fmt.Sprintf("====%d=====", ticketNonce)),
		},
		Parents:               pcids,
		ParentMessageReceipts: c,
		BLSAggregate:          types.Signature{Type: types.KTBLS, Data: []byte("boo! im a signature")},
		ParentWeight:          weight,
		Messages:              c,
		Height:                height,
		ParentStateRoot:       c,
		BlockSig:              &types.Signature{Type: types.KTBLS, Data: []byte("boo! im a signature")},
	}
}

func TipSet(blks ...*types.BlockHeader) *types.TipSet {
	ts, err := types.NewTipSet(blks)
	if err != nil {
		panic(err)
	}
	return ts
}
