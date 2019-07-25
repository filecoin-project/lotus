package gen

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/node/repo"
	block "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("gen")

type ChainGen struct {
	accounts []address.Address

	msgsPerBlock int

	bs blockstore.Blockstore

	cs *store.ChainStore

	genesis *types.BlockHeader

	curBlock *types.FullBlock

	miner address.Address
}

type mybs struct {
	blockstore.Blockstore
}

func (m mybs) Get(c cid.Cid) (block.Block, error) {
	b, err := m.Blockstore.Get(c)
	if err != nil {
		log.Errorf("Get failed: %s %s", c, err)
		return nil, err
	}

	return b, nil
}

func NewGenerator() (*ChainGen, error) {

	mr := repo.NewMemory(nil)
	lr, err := mr.Lock()
	if err != nil {
		return nil, err
	}

	ds, err := lr.Datastore("/blocks")
	if err != nil {
		return nil, err
	}
	bs := mybs{blockstore.NewBlockstore(ds)}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, err
	}

	w, err := wallet.NewWallet(ks)
	if err != nil {
		return nil, err
	}

	miner, err := w.GenerateKey(types.KTBLS)
	if err != nil {
		return nil, err
	}

	banker, err := w.GenerateKey(types.KTBLS)
	if err != nil {
		return nil, err
	}

	genb, err := MakeGenesisBlock(bs, map[address.Address]types.BigInt{
		banker: types.NewInt(90000000),
	})
	if err != nil {
		return nil, err
	}

	cs := store.NewChainStore(bs, ds)

	msgsPerBlock := 10

	genfb := &types.FullBlock{Header: genb.Genesis}

	gen := &ChainGen{
		bs:           bs,
		cs:           cs,
		msgsPerBlock: msgsPerBlock,
		genesis:      genb.Genesis,
		miner:        miner,
		curBlock:     genfb,
	}

	return gen, nil
}

func (cg *ChainGen) Genesis() *types.BlockHeader {
	return cg.genesis
}

func (cg *ChainGen) nextBlockProof() (address.Address, types.ElectionProof, []types.Ticket, error) {
	return cg.miner, []byte("cat in a box"), []types.Ticket{types.Ticket("im a ticket, promise")}, nil
}

func (cg *ChainGen) NextBlock() (*types.FullBlock, error) {
	miner, proof, tickets, err := cg.nextBlockProof()
	if err != nil {
		return nil, err
	}

	var msgs []*types.SignedMessage

	parents, err := types.NewTipSet([]*types.BlockHeader{cg.curBlock.Header})
	if err != nil {
		return nil, err
	}

	fblk, err := MinerCreateBlock(context.TODO(), cg.cs, miner, parents, tickets, proof, msgs)
	if err != nil {
		return nil, err
	}

	cg.curBlock = fblk

	return fblk, nil
}
