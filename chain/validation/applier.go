package validation

import (
	"context"
	"github.com/filecoin-project/chain-validation/pkg/chain"
	"github.com/filecoin-project/chain-validation/pkg/state"
	"github.com/filecoin-project/go-lotus/chain/address"
	lotusstate "github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/types"
	vm "github.com/filecoin-project/go-lotus/chain/vm"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

// Applier applies messages to state trees and storage.
type Applier struct {
}

var _ chain.Applier = &Applier{}

func NewApplier() *Applier {
	return &Applier{}
}

func (a *Applier) ApplyMessage(tree state.Tree, storage state.StorageMap, eCtx *chain.ExecutionContext,
	message interface{}) (state.Tree, chain.MessageReceipt, error) {
	ctx := context.TODO()
	// flushing the state tree feels like it should be idempotent..
	cst := nil // FIXME we need the CborIPLD Store thing.
	base := tree.Cid() //FIXME this will panic since its not implemented. Maybe call tree.Flush in it??
	randSrc := nil // FIXME need chain randomness to construct VM store.NewChainRand().
	bs := blockstore.NewBlockstore(nil) // FIXME this should be the storagemap, I think.
	minerAddr, err := address.NewFromBytes([]byte(eCtx.MinerOwner))
	if err != nil {
		return nil, chain.MessageReceipt{}, err
	}
	lotusVM, err := vm.NewVM(base, eCtx.Epoch, randSrc, minerAddr, bs)
	res, err := lotusVM.ApplyMessage(ctx, message.(*types.Message))
	if err != nil {
		return nil, chain.MessageReceipt{}, err
	}
	lotusstate.LoadStateTree(nil, )
}
