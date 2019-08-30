package gen

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	"github.com/ipfs/go-merkledag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/lib/vdf"
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
	mworker     address.Address
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
		return nil, xerrors.Errorf("taking mem-repo lock failed: %w", err)
	}

	ds, err := lr.Datastore("/metadata")
	if err != nil {
		return nil, xerrors.Errorf("failed to get metadata datastore: %w", err)
	}

	bds, err := lr.Datastore("/blocks")
	if err != nil {
		return nil, xerrors.Errorf("failed to get blocks datastore: %w", err)
	}

	bs := mybs{blockstore.NewIdStore(blockstore.NewBlockstore(bds))}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, xerrors.Errorf("getting repo keystore failed: %w", err)
	}

	w, err := wallet.NewWallet(ks)
	if err != nil {
		return nil, xerrors.Errorf("creating memrepo wallet failed: %w", err)
	}

	worker, err := w.GenerateKey(types.KTBLS)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate worker key: %w", err)
	}

	banker, err := w.GenerateKey(types.KTSecp256k1)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate banker key: %w", err)
	}

	receievers := make([]address.Address, msgsPerBlock)
	for r := range receievers {
		receievers[r], err = w.GenerateKey(types.KTBLS)
		if err != nil {
			return nil, xerrors.Errorf("failed to generate receiver key: %w", err)
		}
	}

	minercfg := &GenMinerCfg{
		Worker: worker,
		Owner:  worker,
	}

	genb, err := MakeGenesisBlock(bs, map[address.Address]types.BigInt{
		worker: types.NewInt(50000),
		banker: types.NewInt(90000000),
	}, minercfg, 100000)
	if err != nil {
		return nil, xerrors.Errorf("make genesis block failed: %w", err)
	}

	cs := store.NewChainStore(bs, ds)

	genfb := &types.FullBlock{Header: genb.Genesis}

	if err := cs.SetGenesis(genb.Genesis); err != nil {
		return nil, xerrors.Errorf("set genesis failed: %w", err)
	}

	if minercfg.MinerAddr == address.Undef {
		return nil, xerrors.Errorf("MakeGenesisBlock failed to set miner address")
	}

	gen := &ChainGen{
		bs:           bs,
		cs:           cs,
		msgsPerBlock: msgsPerBlock,
		genesis:      genb.Genesis,
		w:            w,

		miner:     minercfg.MinerAddr,
		mworker:   worker,
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

func (cg *ChainGen) nextBlockProof(ctx context.Context) (address.Address, types.ElectionProof, []*types.Ticket, error) {

	ticks := cg.curBlock.Header.Tickets
	lastTicket := ticks[len(ticks)-1]

	vrfout, err := ComputeVRF(ctx, cg.w.Sign, cg.mworker, lastTicket.VDFResult)
	if err != nil {
		return address.Undef, nil, nil, err
	}

	out, proof, err := vdf.Run(vrfout)
	if err != nil {
		return address.Undef, nil, nil, err
	}

	tick := &types.Ticket{
		VRFProof:  vrfout,
		VDFProof:  proof,
		VDFResult: out,
	}

	return cg.miner, []byte("cat in a box"), []*types.Ticket{tick}, nil
}

func (cg *ChainGen) NextBlock() (*types.FullBlock, []*types.SignedMessage, error) {
	miner, proof, tickets, err := cg.nextBlockProof(context.TODO())
	if err != nil {
		return nil, nil, err
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
			return nil, nil, err
		}

		sig, err := cg.w.Sign(context.TODO(), cg.banker, unsigned)
		if err != nil {
			return nil, nil, err
		}

		msgs[m] = &types.SignedMessage{
			Message:   msg,
			Signature: *sig,
		}

		if _, err := cg.cs.PutMessage(msgs[m]); err != nil {
			return nil, nil, err
		}
	}

	// create block

	parents, err := types.NewTipSet([]*types.BlockHeader{cg.curBlock.Header})
	if err != nil {
		return nil, nil, err
	}

	ts := parents.MinTimestamp() + (uint64(len(tickets)) * build.BlockDelay)

	fblk, err := MinerCreateBlock(context.TODO(), cg.cs, cg.w, miner, parents, tickets, proof, msgs, ts)
	if err != nil {
		return nil, nil, err
	}

	if err := cg.cs.AddBlock(fblk.Header); err != nil {
		return nil, nil, err
	}

	cg.curBlock = fblk

	return fblk, msgs, nil
}

func (cg *ChainGen) YieldRepo() (repo.Repo, error) {
	if err := cg.lr.Close(); err != nil {
		return nil, err
	}
	return cg.r, nil
}
