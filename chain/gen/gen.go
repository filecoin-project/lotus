package gen

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"sync/atomic"
	"time"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-car"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	genesis2 "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
)

var log = logging.Logger("gen")

const msgsPerBlock = 20

type ChainGen struct {
	msgsPerBlock int

	bs blockstore.Blockstore

	cs *store.ChainStore

	beacon beacon.RandomBeacon

	sm *stmgr.StateManager

	genesis   *types.BlockHeader
	CurTipset *store.FullTipSet

	Timestamper func(*types.TipSet, abi.ChainEpoch) uint64

	GetMessages func(*ChainGen) ([]*types.SignedMessage, error)

	w *wallet.Wallet

	eppProvs    map[address.Address]WinningPoStProver
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

func NewGeneratorWithSectors(numSectors int) (*ChainGen, error) {
	saminer.SupportedProofTypes = map[abi.RegisteredProof]struct{}{
		abi.RegisteredProof_StackedDRG2KiBSeal: {},
	}

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

	banker, err := w.GenerateKey(crypto.SigTypeSecp256k1)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate banker key: %w", err)
	}

	receievers := make([]address.Address, msgsPerBlock)
	for r := range receievers {
		receievers[r], err = w.GenerateKey(crypto.SigTypeBLS)
		if err != nil {
			return nil, xerrors.Errorf("failed to generate receiver key: %w", err)
		}
	}

	maddr1 := genesis2.MinerAddress(0)

	m1temp, err := ioutil.TempDir("", "preseal")
	if err != nil {
		return nil, err
	}

	genm1, k1, err := seed.PreSeal(maddr1, abi.RegisteredProof_StackedDRG2KiBPoSt, 0, numSectors, m1temp, []byte("some randomness"), nil)
	if err != nil {
		return nil, err
	}

	maddr2 := genesis2.MinerAddress(1)

	m2temp, err := ioutil.TempDir("", "preseal")
	if err != nil {
		return nil, err
	}

	genm2, k2, err := seed.PreSeal(maddr2, abi.RegisteredProof_StackedDRG2KiBPoSt, 0, numSectors, m2temp, []byte("some randomness"), nil)
	if err != nil {
		return nil, err
	}

	mk1, err := w.Import(k1)
	if err != nil {
		return nil, err
	}
	mk2, err := w.Import(k2)
	if err != nil {
		return nil, err
	}

	sys := vm.Syscalls(&genFakeVerifier{})

	tpl := genesis.Template{
		Accounts: []genesis.Actor{
			{
				Type:    genesis.TAccount,
				Balance: types.FromFil(40000),
				Meta:    (&genesis.AccountMeta{Owner: mk1}).ActorMeta(),
			},
			{
				Type:    genesis.TAccount,
				Balance: types.FromFil(40000),
				Meta:    (&genesis.AccountMeta{Owner: mk2}).ActorMeta(),
			},
			{
				Type:    genesis.TAccount,
				Balance: types.FromFil(50000),
				Meta:    (&genesis.AccountMeta{Owner: banker}).ActorMeta(),
			},
		},
		Miners: []genesis.Miner{
			*genm1,
			*genm2,
		},
		NetworkName: "",
		Timestamp:   uint64(time.Now().Add(-500 * build.BlockDelay * time.Second).Unix()),
	}

	genb, err := genesis2.MakeGenesisBlock(context.TODO(), bs, sys, tpl)
	if err != nil {
		return nil, xerrors.Errorf("make genesis block failed: %w", err)
	}

	cs := store.NewChainStore(bs, ds, sys)

	genfb := &types.FullBlock{Header: genb.Genesis}
	gents := store.NewFullTipSet([]*types.FullBlock{genfb})

	if err := cs.SetGenesis(genb.Genesis); err != nil {
		return nil, xerrors.Errorf("set genesis failed: %w", err)
	}

	mgen := make(map[address.Address]WinningPoStProver)
	for i := range tpl.Miners {
		mgen[genesis2.MinerAddress(uint64(i))] = &wppProvider{}
	}

	sm := stmgr.NewStateManager(cs)

	miners := []address.Address{maddr1, maddr2}

	beac := beacon.NewMockBeacon(time.Second)
	//beac, err := drand.NewDrandBeacon(tpl.Timestamp, build.BlockDelay)
	//if err != nil {
	//return nil, xerrors.Errorf("creating drand beacon: %w", err)
	//}

	gen := &ChainGen{
		bs:           bs,
		cs:           cs,
		sm:           sm,
		msgsPerBlock: msgsPerBlock,
		genesis:      genb.Genesis,
		beacon:       beac,
		w:            w,

		GetMessages: getRandomMessages,
		Miners:      miners,
		eppProvs:    mgen,
		banker:      banker,
		receivers:   receievers,

		CurTipset: gents,

		r:  mr,
		lr: lr,
	}

	return gen, nil
}

func NewGenerator() (*ChainGen, error) {
	return NewGeneratorWithSectors(1)
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

	if err := car.WriteCarWithWalker(context.TODO(), dserv, []cid.Cid{cg.Genesis().Cid()}, out, CarWalkFunc); err != nil {
		return nil, xerrors.Errorf("genesis car write car failed: %w", err)
	}

	return out.Bytes(), nil
}

func CarWalkFunc(nd format.Node) (out []*format.Link, err error) {
	for _, link := range nd.Links() {
		if link.Cid.Prefix().MhType == uint64(commcid.FC_SEALED_V1) || link.Cid.Prefix().MhType == uint64(commcid.FC_UNSEALED_V1) {
			continue
		}
		out = append(out, link)
	}

	return out, nil
}

func (cg *ChainGen) nextBlockProof(ctx context.Context, pts *types.TipSet, m address.Address, round abi.ChainEpoch) ([]types.BeaconEntry, *types.ElectionProof, *types.Ticket, error) {
	mc := &mca{w: cg.w, sm: cg.sm, pv: ffiwrapper.ProofVerifier, bcn: cg.beacon}

	mbi, err := mc.MinerGetBaseInfo(ctx, m, round, pts.Key())
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("get miner base info: %w", err)
	}

	prev := mbi.PrevBeaconEntry

	entries, err := beacon.BeaconEntriesForBlock(ctx, cg.beacon, round, prev)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("get beacon entries for block: %w", err)
	}

	rbase := prev
	if len(entries) > 0 {
		rbase = entries[len(entries)-1]
	}

	eproof, err := IsRoundWinner(ctx, pts, round, m, rbase, mbi, mc)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("checking round winner failed: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := m.MarshalCBOR(buf); err != nil {
		return nil, nil, nil, xerrors.Errorf("failed to cbor marshal address: %w", err)
	}

	if len(entries) == 0 {
		buf.Write(pts.MinTicket().VRFProof)
	}

	ticketRand, err := store.DrawRandomness(rbase.Data, crypto.DomainSeparationTag_TicketProduction, round-build.TicketRandomnessLookback, buf.Bytes())
	if err != nil {
		return nil, nil, nil, err
	}

	st := pts.ParentState()

	worker, err := stmgr.GetMinerWorkerRaw(ctx, cg.sm, st, m)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("get miner worker: %w", err)
	}

	vrfout, err := ComputeVRF(ctx, cg.w.Sign, worker, ticketRand)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("compute VRF: %w", err)
	}

	return entries, eproof, &types.Ticket{VRFProof: vrfout}, nil
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

	for round := base.Height() + 1; len(blks) == 0; round++ {
		for _, m := range miners {
			bvals, et, ticket, err := cg.nextBlockProof(context.TODO(), base, m, round)
			if err != nil {
				return nil, xerrors.Errorf("next block proof: %w", err)
			}

			if et != nil {
				// TODO: maybe think about passing in more real parameters to this?
				wpost, err := cg.eppProvs[m].ComputeProof(context.TODO(), nil, nil)
				if err != nil {
					return nil, err
				}

				fblk, err := cg.makeBlock(base, m, ticket, et, bvals, round, wpost, msgs)
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

func (cg *ChainGen) makeBlock(parents *types.TipSet, m address.Address, vrfticket *types.Ticket,
	eticket *types.ElectionProof, bvals []types.BeaconEntry, height abi.ChainEpoch,
	wpost []abi.PoStProof, msgs []*types.SignedMessage) (*types.FullBlock, error) {

	var ts uint64
	if cg.Timestamper != nil {
		ts = cg.Timestamper(parents, height-parents.Height())
	} else {
		ts = parents.MinTimestamp() + uint64((height-parents.Height())*build.BlockDelay)
	}

	fblk, err := MinerCreateBlock(context.TODO(), cg.sm, cg.w, &api.BlockTemplate{
		Miner:            m,
		Parents:          parents.Key(),
		Ticket:           vrfticket,
		Eproof:           eticket,
		BeaconValues:     bvals,
		Messages:         msgs,
		Epoch:            height,
		Timestamp:        ts,
		WinningPoStProof: wpost,
	})
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

			GasLimit: 10000,
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
	ChainGetRandomness(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)

	MinerGetBaseInfo(context.Context, address.Address, abi.ChainEpoch, types.TipSetKey) (*api.MiningBaseInfo, error)

	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
}

type mca struct {
	w   *wallet.Wallet
	sm  *stmgr.StateManager
	pv  ffiwrapper.Verifier
	bcn beacon.RandomBeacon
}

func (mca mca) ChainGetRandomness(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	pts, err := mca.sm.ChainStore().LoadTipSet(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset key: %w", err)
	}

	return mca.sm.ChainStore().GetRandomness(ctx, pts.Cids(), personalization, randEpoch, entropy)
}

func (mca mca) MinerGetBaseInfo(ctx context.Context, maddr address.Address, epoch abi.ChainEpoch, tsk types.TipSetKey) (*api.MiningBaseInfo, error) {
	return stmgr.MinerGetBaseInfo(ctx, mca.sm, mca.bcn, tsk, epoch, maddr, mca.pv)
}

func (mca mca) WalletSign(ctx context.Context, a address.Address, v []byte) (*crypto.Signature, error) {
	return mca.w.Sign(ctx, a, v)
}

type WinningPoStProver interface {
	GenerateCandidates(context.Context, abi.PoStRandomness, uint64) ([]uint64, error)
	ComputeProof(context.Context, []abi.SectorInfo, abi.PoStRandomness) ([]abi.PoStProof, error)
}

type wppProvider struct{}

func (wpp *wppProvider) GenerateCandidates(ctx context.Context, _ abi.PoStRandomness, _ uint64) ([]uint64, error) {
	return []uint64{0}, nil
}

func (wpp *wppProvider) ComputeProof(context.Context, []abi.SectorInfo, abi.PoStRandomness) ([]abi.PoStProof, error) {
	return []abi.PoStProof{{
		ProofBytes: []byte("valid proof"),
	}}, nil
}

type ProofInput struct {
	sectors           []abi.SectorInfo
	hvrf              []byte
	challengedSectors []uint64
	vrfout            []byte
}

func IsRoundWinner(ctx context.Context, ts *types.TipSet, round abi.ChainEpoch,
	miner address.Address, brand types.BeaconEntry, mbi *api.MiningBaseInfo, a MiningCheckAPI) (*types.ElectionProof, error) {

	buf := new(bytes.Buffer)
	if err := miner.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to cbor marshal address: %w")
	}

	electionRand, err := store.DrawRandomness(brand.Data, crypto.DomainSeparationTag_ElectionProofProduction, round, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to draw randomness: %w", err)
	}

	vrfout, err := ComputeVRF(ctx, a.WalletSign, mbi.WorkerKey, electionRand)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute VRF: %w", err)
	}

	// TODO: wire in real power
	if !types.IsTicketWinner(vrfout, mbi.MinerPower, mbi.NetworkPower) {
		return nil, nil
	}

	return &types.ElectionProof{VRFProof: vrfout}, nil
}

type SignFunc func(context.Context, address.Address, []byte) (*crypto.Signature, error)

func VerifyVRF(ctx context.Context, worker address.Address, vrfBase, vrfproof []byte) error {
	_, span := trace.StartSpan(ctx, "VerifyVRF")
	defer span.End()

	sig := &crypto.Signature{
		Type: crypto.SigTypeBLS,
		Data: vrfproof,
	}

	if err := sigs.Verify(sig, worker, vrfBase); err != nil {
		return xerrors.Errorf("vrf was invalid: %w", err)
	}

	return nil
}

func ComputeVRF(ctx context.Context, sign SignFunc, worker address.Address, sigInput []byte) ([]byte, error) {
	sig, err := sign(ctx, worker, sigInput)
	if err != nil {
		return nil, err
	}

	if sig.Type != crypto.SigTypeBLS {
		return nil, fmt.Errorf("miner worker address was not a BLS key")
	}

	return sig.Data, nil
}

type genFakeVerifier struct{}

var _ ffiwrapper.Verifier = (*genFakeVerifier)(nil)

func (m genFakeVerifier) VerifySeal(svi abi.SealVerifyInfo) (bool, error) {
	return true, nil
}

func (m genFakeVerifier) VerifyWinningPoSt(ctx context.Context, info abi.WinningPoStVerifyInfo) (bool, error) {
	panic("not supported")
}

func (m genFakeVerifier) VerifyWindowPoSt(ctx context.Context, info abi.WindowPoStVerifyInfo) (bool, error) {
	panic("not supported")
}

func (m genFakeVerifier) GenerateWinningPoStSectorChallenge(ctx context.Context, proof abi.RegisteredProof, id abi.ActorID, randomness abi.PoStRandomness, u uint64) ([]uint64, error) {
	panic("not supported")
}
