package gen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"sync/atomic"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipfs/go-merkledag"
	peer "github.com/libp2p/go-libp2p-core/peer"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/node/repo"

	"go.opencensus.io/trace"
	"golang.org/x/xerrors"
)

var log = logging.Logger("gen")

const msgsPerBlock = 20

type ChainGen struct {
	msgsPerBlock int

	bs blockstore.Blockstore

	cs *store.ChainStore

	sm *stmgr.StateManager

	genesis   *types.BlockHeader
	CurTipset *store.FullTipSet

	Timestamper func(*types.TipSet, uint64) uint64

	GetMessages func(*ChainGen) ([]*types.SignedMessage, error)

	w *wallet.Wallet

	eppProvs    map[address.Address]ElectionPoStProver
	Miners      []address.Address
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
		return nil, err
	}

	return b, nil
}

func NewGenerator() (*ChainGen, error) {
	mr := repo.NewMemory(nil)
	lr, err := mr.Lock(repo.StorageMiner)
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

	maddr1, err := address.NewFromString("t0300")
	if err != nil {
		return nil, err
	}

	m1temp, err := ioutil.TempDir("", "preseal")
	if err != nil {
		return nil, err
	}

	genm1, err := seed.PreSeal(maddr1, 1024, 0, 1, m1temp, []byte("some randomness"))
	if err != nil {
		return nil, err
	}

	maddr2, err := address.NewFromString("t0301")
	if err != nil {
		return nil, err
	}

	m2temp, err := ioutil.TempDir("", "preseal")
	if err != nil {
		return nil, err
	}

	genm2, err := seed.PreSeal(maddr2, 1024, 0, 1, m2temp, []byte("some randomness"))
	if err != nil {
		return nil, err
	}

	mk1, err := w.Import(&genm1.Key)
	if err != nil {
		return nil, err
	}
	mk2, err := w.Import(&genm2.Key)
	if err != nil {
		return nil, err
	}

	minercfg := &GenMinerCfg{
		PeerIDs: []peer.ID{"peerID1", "peerID2"},
		PreSeals: map[string]genesis.GenesisMiner{
			maddr1.String(): *genm1,
			maddr2.String(): *genm2,
		},
		MinerAddrs: []address.Address{maddr1, maddr2},
	}

	sys := vm.Syscalls(sectorbuilder.ProofVerifier)

	genb, err := MakeGenesisBlock(bs, sys, map[address.Address]types.BigInt{
		mk1:    types.FromFil(40000),
		mk2:    types.FromFil(40000),
		banker: types.FromFil(50000),
	}, minercfg, 100000)
	if err != nil {
		return nil, xerrors.Errorf("make genesis block failed: %w", err)
	}

	cs := store.NewChainStore(bs, ds, sys)

	genfb := &types.FullBlock{Header: genb.Genesis}
	gents := store.NewFullTipSet([]*types.FullBlock{genfb})

	if err := cs.SetGenesis(genb.Genesis); err != nil {
		return nil, xerrors.Errorf("set genesis failed: %w", err)
	}

	if len(minercfg.MinerAddrs) == 0 {
		return nil, xerrors.Errorf("MakeGenesisBlock failed to set miner address")
	}

	mgen := make(map[address.Address]ElectionPoStProver)
	for _, m := range minercfg.MinerAddrs {
		mgen[m] = &eppProvider{}
	}

	sm := stmgr.NewStateManager(cs)

	gen := &ChainGen{
		bs:           bs,
		cs:           cs,
		sm:           sm,
		msgsPerBlock: msgsPerBlock,
		genesis:      genb.Genesis,
		w:            w,

		GetMessages: getRandomMessages,
		Miners:      minercfg.MinerAddrs,
		eppProvs:    mgen,
		banker:      banker,
		receivers:   receievers,

		CurTipset: gents,

		r:  mr,
		lr: lr,
	}

	return gen, nil
}

func (cg *ChainGen) SetStateManager(sm *stmgr.StateManager) {
	cg.sm = sm
}

func (cg *ChainGen) ChainStore() *store.ChainStore {
	return cg.cs
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
		return nil, xerrors.Errorf("genesis car write car failed: %w", err)
	}

	return out.Bytes(), nil
}

func (cg *ChainGen) nextBlockProof(ctx context.Context, pts *types.TipSet, m address.Address, round int64) (*types.EPostProof, *types.Ticket, error) {

	lastTicket := pts.MinTicket()

	st := pts.ParentState()

	worker, err := stmgr.GetMinerWorkerRaw(ctx, cg.sm, st, m)
	if err != nil {
		return nil, nil, xerrors.Errorf("get miner worker: %w", err)
	}

	vrfout, err := ComputeVRF(ctx, cg.w.Sign, worker, m, DSepTicket, lastTicket.VRFProof)
	if err != nil {
		return nil, nil, xerrors.Errorf("compute VRF: %w", err)
	}

	tick := &types.Ticket{
		VRFProof: vrfout,
	}

	eproofin, err := IsRoundWinner(ctx, pts, round, m, cg.eppProvs[m], &mca{w: cg.w, sm: cg.sm})
	if err != nil {
		return nil, nil, xerrors.Errorf("checking round winner failed: %w", err)
	}
	if eproofin == nil {
		return nil, tick, nil
	}
	eproof, err := ComputeProof(ctx, cg.eppProvs[m], eproofin)
	if err != nil {
		return nil, nil, xerrors.Errorf("computing proof: %w", err)
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

	msgs, err := cg.GetMessages(cg)
	if err != nil {
		return nil, xerrors.Errorf("get random messages: %w", err)
	}

	for round := int64(base.Height() + 1); len(blks) == 0; round++ {
		for _, m := range miners {
			proof, t, err := cg.nextBlockProof(context.TODO(), base, m, round)
			if err != nil {
				return nil, xerrors.Errorf("next block proof: %w", err)
			}

			if proof != nil {
				fblk, err := cg.makeBlock(base, m, proof, t, uint64(round), msgs)
				if err != nil {
					return nil, xerrors.Errorf("making a block for next tipset failed: %w", err)
				}

				if err := cg.cs.PersistBlockHeaders(fblk.Header); err != nil {
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

func (cg *ChainGen) makeBlock(parents *types.TipSet, m address.Address, eproof *types.EPostProof, ticket *types.Ticket, height uint64, msgs []*types.SignedMessage) (*types.FullBlock, error) {

	var ts uint64
	if cg.Timestamper != nil {
		ts = cg.Timestamper(parents, height-parents.Height())
	} else {
		ts = parents.MinTimestamp() + ((height - parents.Height()) * build.BlockDelay)
	}

	fblk, err := MinerCreateBlock(context.TODO(), cg.sm, cg.w, m, parents, ticket, eproof, msgs, height, ts)
	if err != nil {
		return nil, err
	}

	return fblk, err
}

// This function is awkward. It's used to deal with messages made when
// simulating forks
func (cg *ChainGen) ResyncBankerNonce(ts *types.TipSet) error {
	act, err := cg.sm.GetActor(cg.banker, ts)
	if err != nil {
		return err
	}

	cg.bankerNonce = act.Nonce
	return nil
}

func (cg *ChainGen) Banker() address.Address {
	return cg.banker
}

func (cg *ChainGen) Wallet() *wallet.Wallet {
	return cg.w
}

func getRandomMessages(cg *ChainGen) ([]*types.SignedMessage, error) {
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

		sig, err := cg.w.Sign(context.TODO(), cg.banker, msg.Cid().Bytes())
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
	ChainGetRandomness(context.Context, types.TipSetKey, int64) ([]byte, error)

	StateMinerPower(context.Context, address.Address, types.TipSetKey) (api.MinerPower, error)

	StateMinerWorker(context.Context, address.Address, types.TipSetKey) (address.Address, error)

	StateMinerSectorSize(context.Context, address.Address, types.TipSetKey) (uint64, error)

	StateMinerProvingSet(context.Context, address.Address, types.TipSetKey) ([]*api.ChainSectorInfo, error)

	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
}

type mca struct {
	w  *wallet.Wallet
	sm *stmgr.StateManager
}

func (mca mca) ChainGetRandomness(ctx context.Context, pts types.TipSetKey, lb int64) ([]byte, error) {
	return mca.sm.ChainStore().GetRandomness(ctx, pts.Cids(), int64(lb))
}

func (mca mca) StateMinerPower(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (api.MinerPower, error) {
	ts, err := mca.sm.ChainStore().LoadTipSet(tsk)
	if err != nil {
		return api.MinerPower{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	mpow, tpow, err := stmgr.GetPower(ctx, mca.sm, ts, maddr)
	if err != nil {
		return api.MinerPower{}, err
	}

	return api.MinerPower{
		MinerPower: mpow,
		TotalPower: tpow,
	}, err
}

func (mca mca) StateMinerWorker(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := mca.sm.ChainStore().LoadTipSet(tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerWorkerRaw(ctx, mca.sm, ts.ParentState(), maddr)
}

func (mca mca) StateMinerSectorSize(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (uint64, error) {
	ts, err := mca.sm.ChainStore().LoadTipSet(tsk)
	if err != nil {
		return 0, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerSectorSize(ctx, mca.sm, ts, maddr)
}

func (mca mca) StateMinerProvingSet(ctx context.Context, maddr address.Address, tsk types.TipSetKey) ([]*api.ChainSectorInfo, error) {
	ts, err := mca.sm.ChainStore().LoadTipSet(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerProvingSet(ctx, mca.sm, ts, maddr)
}

func (mca mca) WalletSign(ctx context.Context, a address.Address, v []byte) (*types.Signature, error) {
	return mca.w.Sign(ctx, a, v)
}

type ElectionPoStProver interface {
	GenerateCandidates(context.Context, sectorbuilder.SortedPublicSectorInfo, []byte) ([]sectorbuilder.EPostCandidate, error)
	ComputeProof(context.Context, sectorbuilder.SortedPublicSectorInfo, []byte, []sectorbuilder.EPostCandidate) ([]byte, error)
}

type eppProvider struct{}

func (epp *eppProvider) GenerateCandidates(ctx context.Context, _ sectorbuilder.SortedPublicSectorInfo, eprand []byte) ([]sectorbuilder.EPostCandidate, error) {
	return []sectorbuilder.EPostCandidate{
		{
			SectorID:             1,
			PartialTicket:        [32]byte{},
			Ticket:               [32]byte{},
			SectorChallengeIndex: 1,
		},
	}, nil
}

func (epp *eppProvider) ComputeProof(ctx context.Context, _ sectorbuilder.SortedPublicSectorInfo, eprand []byte, winners []sectorbuilder.EPostCandidate) ([]byte, error) {

	return []byte("valid proof"), nil
}

type ProofInput struct {
	sectors sectorbuilder.SortedPublicSectorInfo
	hvrf    []byte
	winners []sectorbuilder.EPostCandidate
	vrfout  []byte
}

func IsRoundWinner(ctx context.Context, ts *types.TipSet, round int64, miner address.Address, epp ElectionPoStProver, a MiningCheckAPI) (*ProofInput, error) {
	r, err := a.ChainGetRandomness(ctx, ts.Key(), round-build.EcRandomnessLookback)
	if err != nil {
		return nil, xerrors.Errorf("chain get randomness: %w", err)
	}

	mworker, err := a.StateMinerWorker(ctx, miner, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get miner worker: %w", err)
	}

	vrfout, err := ComputeVRF(ctx, a.WalletSign, mworker, miner, DSepElectionPost, r)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute VRF: %w", err)
	}

	pset, err := a.StateMinerProvingSet(ctx, miner, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to load proving set for miner: %w", err)
	}
	if len(pset) == 0 {
		return nil, nil
	}

	var sinfos []ffi.PublicSectorInfo
	for _, s := range pset {
		var commRa [32]byte
		copy(commRa[:], s.CommR)
		sinfos = append(sinfos, ffi.PublicSectorInfo{
			SectorID: s.SectorID,
			CommR:    commRa,
		})
	}
	sectors := sectorbuilder.NewSortedPublicSectorInfo(sinfos)

	hvrf := sha256.Sum256(vrfout)
	candidates, err := epp.GenerateCandidates(ctx, sectors, hvrf[:])
	if err != nil {
		return nil, xerrors.Errorf("failed to generate electionPoSt candidates: %w", err)
	}

	pow, err := a.StateMinerPower(ctx, miner, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to check power: %w", err)
	}

	ssize, err := a.StateMinerSectorSize(ctx, miner, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to look up miners sector size: %w", err)
	}

	var winners []sectorbuilder.EPostCandidate
	for _, c := range candidates {
		if types.IsTicketWinner(c.PartialTicket[:], ssize, uint64(len(sinfos)), pow.TotalPower) {
			winners = append(winners, c)
		}
	}

	// no winners, sad
	if len(winners) == 0 {
		return nil, nil
	}

	return &ProofInput{
		sectors: sectors,
		hvrf:    hvrf[:],
		winners: winners,
		vrfout:  vrfout,
	}, nil
}

func ComputeProof(ctx context.Context, epp ElectionPoStProver, pi *ProofInput) (*types.EPostProof, error) {
	proof, err := epp.ComputeProof(ctx, pi.sectors, pi.hvrf, pi.winners)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute snark for election proof: %w", err)
	}

	ept := types.EPostProof{
		Proof:    proof,
		PostRand: pi.vrfout,
	}
	for _, win := range pi.winners {
		part := make([]byte, 32)
		copy(part, win.PartialTicket[:])
		ept.Candidates = append(ept.Candidates, types.EPostTicket{
			Partial:        part,
			SectorID:       win.SectorID,
			ChallengeIndex: win.SectorChallengeIndex,
		})
	}

	return &ept, nil
}

type SignFunc func(context.Context, address.Address, []byte) (*types.Signature, error)

const (
	DSepTicket       = 1
	DSepElectionPost = 2
)

func hashVRFBase(personalization uint64, miner address.Address, input []byte) ([]byte, error) {
	if miner.Protocol() != address.ID {
		return nil, xerrors.Errorf("miner address for compute VRF must be an ID address")
	}

	var persbuf [8]byte
	binary.LittleEndian.PutUint64(persbuf[:], personalization)

	h := sha256.New()
	h.Write(persbuf[:])
	h.Write([]byte{0})
	h.Write(input)
	h.Write([]byte{0})
	h.Write(miner.Bytes())

	return h.Sum(nil), nil
}

func VerifyVRF(ctx context.Context, worker, miner address.Address, p uint64, input, vrfproof []byte) error {
	_, span := trace.StartSpan(ctx, "VerifyVRF")
	defer span.End()

	vrfBase, err := hashVRFBase(p, miner, input)
	if err != nil {
		return xerrors.Errorf("computing vrf base failed: %w", err)
	}

	sig := &types.Signature{
		Type: types.KTBLS,
		Data: vrfproof,
	}

	if err := sigs.Verify(sig, worker, vrfBase); err != nil {
		return xerrors.Errorf("vrf was invalid: %w", err)
	}

	return nil
}

func ComputeVRF(ctx context.Context, sign SignFunc, worker, miner address.Address, p uint64, input []byte) ([]byte, error) {
	sigInput, err := hashVRFBase(p, miner, input)
	if err != nil {
		return nil, err
	}

	sig, err := sign(ctx, worker, sigInput)
	if err != nil {
		return nil, err
	}

	if sig.Type != types.KTBLS {
		return nil, fmt.Errorf("miner worker address was not a BLS key")
	}

	return sig.Data, nil
}
