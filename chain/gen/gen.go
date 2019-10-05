package gen

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	"github.com/ipfs/go-merkledag"
	peer "github.com/libp2p/go-libp2p-peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/stmgr"
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

	sm *stmgr.StateManager

	genesis   *types.BlockHeader
	CurTipset *store.FullTipSet

	w *wallet.Wallet

	Miners      []address.Address
	mworkers    []address.Address
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
		// TODO: debug why we get so many not founds in tests
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
		Workers: []address.Address{worker, worker},
		Owners:  []address.Address{worker, worker},
		PeerIDs: []peer.ID{"peerID1", "peerID2"},
	}

	genb, err := MakeGenesisBlock(bs, map[address.Address]types.BigInt{
		worker: types.FromFil(40000),
		banker: types.FromFil(50000),
	}, minercfg, 100000)
	if err != nil {
		return nil, xerrors.Errorf("make genesis block failed: %w", err)
	}

	cs := store.NewChainStore(bs, ds)

	genfb := &types.FullBlock{Header: genb.Genesis}
	gents := store.NewFullTipSet([]*types.FullBlock{genfb})

	if err := cs.SetGenesis(genb.Genesis); err != nil {
		return nil, xerrors.Errorf("set genesis failed: %w", err)
	}

	if len(minercfg.MinerAddrs) == 0 {
		return nil, xerrors.Errorf("MakeGenesisBlock failed to set miner address")
	}

	sm := stmgr.NewStateManager(cs)

	gen := &ChainGen{
		bs:           bs,
		cs:           cs,
		sm:           sm,
		msgsPerBlock: msgsPerBlock,
		genesis:      genb.Genesis,
		w:            w,

		Miners:    minercfg.MinerAddrs,
		mworkers:  minercfg.Workers,
		banker:    banker,
		receivers: receievers,

		CurTipset: gents,

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

func (cg *ChainGen) nextBlockProof(ctx context.Context, m address.Address, ticks []*types.Ticket) (types.ElectionProof, *types.Ticket, error) {
	pts := cg.CurTipset.TipSet()

	var lastTicket *types.Ticket
	if len(ticks) == 0 {
		lastTicket = pts.MinTicket()
	} else {
		lastTicket = ticks[len(ticks)-1]
	}

	st := cg.CurTipset.TipSet().ParentState()

	worker, err := stmgr.GetMinerWorker(ctx, cg.sm, st, m)
	if err != nil {
		return nil, nil, err
	}

	vrfout, err := ComputeVRF(ctx, cg.w.Sign, worker, lastTicket.VDFResult)
	if err != nil {
		return nil, nil, err
	}

	out, proof, err := vdf.Run(vrfout)
	if err != nil {
		return nil, nil, err
	}

	tick := &types.Ticket{
		VRFProof:  vrfout,
		VDFProof:  proof,
		VDFResult: out,
	}

	win, eproof, err := IsRoundWinner(ctx, pts, append(ticks, tick), m, &mca{w: cg.w, sm: cg.sm})
	if err != nil {
		return nil, nil, xerrors.Errorf("checking round winner failed: %w", err)
	}
	if !win {
		return nil, tick, nil
	}

	return eproof, tick, nil
}

type MinedTipSet struct {
	TipSet   *store.FullTipSet
	Messages []*types.SignedMessage
}

func (cg *ChainGen) NextTipSet() (*MinedTipSet, error) {
	mts, err := cg.NextTipSetFromMiners(cg.CurTipset.TipSet(), cg.Miners)
	if err != nil {
		return nil, err
	}

	cg.CurTipset = mts.TipSet
	return mts, nil
}

func (cg *ChainGen) NextTipSetFromMiners(base *types.TipSet, miners []address.Address) (*MinedTipSet, error) {
	var blks []*types.FullBlock
	ticketSets := make([][]*types.Ticket, len(miners))

	msgs, err := cg.getRandomMessages()
	if err != nil {
		return nil, err
	}

	for len(blks) == 0 {
		for i, m := range miners {
			proof, t, err := cg.nextBlockProof(context.TODO(), m, ticketSets[i])
			if err != nil {
				return nil, xerrors.Errorf("next block proof: %w", err)
			}

			ticketSets[i] = append(ticketSets[i], t)
			if proof != nil {
				fblk, err := cg.makeBlock(base, m, proof, ticketSets[i], msgs)
				if err != nil {
					return nil, xerrors.Errorf("making a block for next tipset failed: %w", err)
				}

				if err := cg.cs.PersistBlockHeader(fblk.Header); err != nil {
					return nil, xerrors.Errorf("chainstore AddBlock: %w", err)
				}

				blks = append(blks, fblk)
			}
		}
	}

	fts := store.NewFullTipSet(blks)

	return &MinedTipSet{
		TipSet:   fts,
		Messages: msgs,
	}, nil
}

func (cg *ChainGen) makeBlock(parents *types.TipSet, m address.Address, eproof types.ElectionProof, tickets []*types.Ticket, msgs []*types.SignedMessage) (*types.FullBlock, error) {

	ts := parents.MinTimestamp() + (uint64(len(tickets)) * build.BlockDelay)

	fblk, err := MinerCreateBlock(context.TODO(), cg.sm, cg.w, m, parents, tickets, eproof, msgs, ts)
	if err != nil {
		return nil, err
	}

	return fblk, err
}

func (cg *ChainGen) getRandomMessages() ([]*types.SignedMessage, error) {
	msgs := make([]*types.SignedMessage, cg.msgsPerBlock)
	for m := range msgs {
		msg := types.Message{
			To:   cg.receivers[m%len(cg.receivers)],
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

		sig, err := cg.w.Sign(context.TODO(), cg.banker, unsigned)
		if err != nil {
			return nil, err
		}

		msgs[m] = &types.SignedMessage{
			Message:   msg,
			Signature: *sig,
		}
	}

	return msgs, nil
}

func (cg *ChainGen) YieldRepo() (repo.Repo, error) {
	if err := cg.lr.Close(); err != nil {
		return nil, err
	}
	return cg.r, nil
}

type MiningCheckAPI interface {
	ChainGetRandomness(context.Context, *types.TipSet, []*types.Ticket, int) ([]byte, error)

	StateMinerPower(context.Context, address.Address, *types.TipSet) (api.MinerPower, error)

	StateMinerWorker(context.Context, address.Address, *types.TipSet) (address.Address, error)

	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
}

type mca struct {
	w  *wallet.Wallet
	sm *stmgr.StateManager
}

func (mca mca) ChainGetRandomness(ctx context.Context, pts *types.TipSet, ticks []*types.Ticket, lb int) ([]byte, error) {
	return mca.sm.ChainStore().GetRandomness(ctx, pts.Cids(), ticks, int64(lb))
}

func (mca mca) StateMinerPower(ctx context.Context, maddr address.Address, ts *types.TipSet) (api.MinerPower, error) {
	mpow, tpow, err := stmgr.GetPower(ctx, mca.sm, ts, maddr)
	if err != nil {
		return api.MinerPower{}, err
	}

	return api.MinerPower{
		MinerPower: mpow,
		TotalPower: tpow,
	}, err
}

func (mca mca) StateMinerWorker(ctx context.Context, maddr address.Address, ts *types.TipSet) (address.Address, error) {
	return stmgr.GetMinerWorker(ctx, mca.sm, ts.ParentState(), maddr)
}

func (mca mca) WalletSign(ctx context.Context, a address.Address, v []byte) (*types.Signature, error) {
	return mca.w.Sign(ctx, a, v)
}

func IsRoundWinner(ctx context.Context, ts *types.TipSet, ticks []*types.Ticket, miner address.Address, a MiningCheckAPI) (bool, types.ElectionProof, error) {
	r, err := a.ChainGetRandomness(ctx, ts, ticks, build.RandomnessLookback)
	if err != nil {
		return false, nil, err
	}

	mworker, err := a.StateMinerWorker(ctx, miner, ts)
	if err != nil {
		return false, nil, xerrors.Errorf("failed to get miner worker: %w", err)
	}

	vrfout, err := ComputeVRF(ctx, a.WalletSign, mworker, r)
	if err != nil {
		return false, nil, xerrors.Errorf("failed to compute VRF: %w", err)
	}

	pow, err := a.StateMinerPower(ctx, miner, ts)
	if err != nil {
		return false, nil, xerrors.Errorf("failed to check power: %w", err)
	}

	return types.PowerCmp(vrfout, pow.MinerPower, pow.TotalPower), vrfout, nil
}

type SignFunc func(context.Context, address.Address, []byte) (*types.Signature, error)

func ComputeVRF(ctx context.Context, sign SignFunc, w address.Address, input []byte) ([]byte, error) {
	sig, err := sign(ctx, w, input)
	if err != nil {
		return nil, err
	}

	if sig.Type != types.KTBLS {
		return nil, fmt.Errorf("miner worker address was not a BLS key")
	}

	return sig.Data, nil
}
