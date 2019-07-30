package gen

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	"github.com/ipfs/go-merkledag"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/node/repo"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("gen")

const msgsPerBlock = 20

type ChainGen struct {
	accounts []address.Address

	msgsPerBlock int

	bs blockstore.Blockstore

	cs *store.ChainStore

	genesis  *types.BlockHeader
	curBlock *types.FullBlock

	w *wallet.Wallet

	miner       address.Address
	receivers   []address.Address
	banker      address.Address
	bankerNonce uint64

	r  repo.Repo
	lr repo.LockedRepo
}

type mybs struct {
	blockstore.Blockstore
}

func (m mybs) Get(c cid.Cid) (block.Block, error) {
	b, err := m.Blockstore.Get(c)
	if err != nil {
		// change to error for stacktraces, don't commit with that pls
		log.Warnf("Get failed: %s %s", c, err)
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

	ds, err := lr.Datastore("/metadata")
	if err != nil {
		return nil, err
	}

	bds, err := lr.Datastore("/blocks")
	if err != nil {
		return nil, err
	}

	bs := mybs{blockstore.NewIdStore(blockstore.NewBlockstore(bds))}

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

	// KTBLS doesn't support signature verification or something like that yet
	banker, err := w.GenerateKey(types.KTSecp256k1)
	if err != nil {
		return nil, err
	}

	receievers := make([]address.Address, msgsPerBlock)
	for r := range receievers {
		receievers[r], err = w.GenerateKey(types.KTBLS)
		if err != nil {
			return nil, err
		}
	}

	genb, err := MakeGenesisBlock(bs, map[address.Address]types.BigInt{
		miner:  types.NewInt(5),
		banker: types.NewInt(90000000),
	})
	if err != nil {
		return nil, err
	}

	cs := store.NewChainStore(bs, ds)

	genfb := &types.FullBlock{Header: genb.Genesis}

	if err := cs.SetGenesis(genb.Genesis); err != nil {
		return nil, err
	}

	gen := &ChainGen{
		bs:           bs,
		cs:           cs,
		msgsPerBlock: msgsPerBlock,
		genesis:      genb.Genesis,
		w:            w,

		miner:     miner,
		banker:    banker,
		receivers: receievers,

		curBlock: genfb,

		r:  mr,
		lr: lr,
	}

	return gen, nil
}

func (cg *ChainGen) Genesis() *types.BlockHeader {
	return cg.genesis
}

func (cg *ChainGen) GenesisCar() ([]byte, error) {
	offl := offline.Exchange(cg.bs)
	blkserv := blockservice.New(cg.bs, offl)
	dserv := merkledag.NewDAGService(blkserv)

	out := new(bytes.Buffer)

	if err := car.WriteCar(context.TODO(), dserv, []cid.Cid{cg.Genesis().Cid()}, out); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

func (cg *ChainGen) nextBlockProof() (address.Address, types.ElectionProof, []types.Ticket, error) {
	return cg.miner, []byte("cat in a box"), []types.Ticket{types.Ticket("im a ticket, promise")}, nil
}

func (cg *ChainGen) NextBlock() (*types.FullBlock, error) {
	miner, proof, tickets, err := cg.nextBlockProof()
	if err != nil {
		return nil, err
	}

	// make some transfers from banker

	msgs := make([]*types.SignedMessage, cg.msgsPerBlock)
	for m := range msgs {
		msg := types.Message{
			To:   cg.receivers[m],
			From: cg.banker,

			Nonce: atomic.AddUint64(&cg.bankerNonce, 1) - 1,

			Value: types.NewInt(uint64(m + 1)),

			Method: 0,

			GasLimit: types.NewInt(10000),
			GasPrice: types.NewInt(0),
		}

		unsigned, err := msg.Serialize()
		if err != nil {
			return nil, err
		}

		sig, err := cg.w.Sign(cg.banker, unsigned)
		if err != nil {
			return &types.FullBlock{}, err
		}

		msgs[m] = &types.SignedMessage{
			Message:   msg,
			Signature: *sig,
		}

		if _, err := cg.cs.PutMessage(msgs[m]); err != nil {
			return nil, err
		}
	}

	// create block

	parents, err := types.NewTipSet([]*types.BlockHeader{cg.curBlock.Header})
	if err != nil {
		return nil, err
	}

	fblk, err := MinerCreateBlock(context.TODO(), cg.cs, miner, parents, tickets, proof, msgs)
	if err != nil {
		return nil, err
	}

	if err := cg.cs.AddBlock(fblk.Header); err != nil {
		return nil, err
	}

	cg.curBlock = fblk

	return fblk, nil
}

func (cg *ChainGen) YieldRepo() (repo.Repo, error) {
	if err := cg.lr.Close(); err != nil {
		return nil, err
	}
	return cg.r, nil
}
