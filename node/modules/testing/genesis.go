package testing

import (
	blockstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/node/modules"
)

func MakeGenesis(bs blockstore.Blockstore, w *chain.Wallet) (modules.Genesis, error) {
	genb, err := chain.MakeGenesisBlock(bs, w)
	if err != nil {
		return nil, err
	}
	return genb.Genesis, nil
}
