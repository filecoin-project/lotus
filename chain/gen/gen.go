package gen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/boxo/blockservice"
	offline "github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-car"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	genesis2 "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/proofs"
	proofsffi "github.com/filecoin-project/lotus/chain/proofs/ffi"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/node/repo"
)

const msgsPerBlock = 20

//nolint:deadcode,varcheck
var log = logging.Logger("gen")

var ValidWpostForTesting = []proof7.PoStProof{{
	ProofBytes: []byte("valid proof"),
}}

type ChainGen struct {
	msgsPerBlock int

	bs blockstore.Blockstore

	cs *store.ChainStore

	beacon beacon.Schedule

	sm *stmgr.StateManager

	genesis   *types.BlockHeader
	CurTipset *store.FullTipSet

	Timestamper func(*types.TipSet, abi.ChainEpoch) uint64

	GetMessages func(*ChainGen) ([]*types.SignedMessage, error)

	w *wallet.LocalWallet

	eppProvs  map[address.Address]WinningPoStProver
	Miners    []address.Address
	receivers []address.Address
	// a SecP address
	banker      address.Address
	bankerNonce uint64

	r  repo.Repo
	lr repo.LockedRepo
}

var rootkeyMultisig = genesis.MultisigMeta{
	Signers:         []address.Address{remAccTestKey},
	Threshold:       1,
	VestingDuration: 0,
	VestingStart:    0,
}

var DefaultVerifregRootkeyActor = genesis.Actor{
	Type:    genesis.TMultisig,
	Balance: big.NewInt(0),
	Meta:    rootkeyMultisig.ActorMeta(),
}

var remAccTestKey, _ = address.NewFromString("t1ceb34gnsc6qk5dt6n7xg6ycwzasjhbxm3iylkiy")
var remAccMeta = genesis.MultisigMeta{
	Signers:   []address.Address{remAccTestKey},
	Threshold: 1,
}

var DefaultRemainderAccountActor = genesis.Actor{
	Type:    genesis.TMultisig,
	Balance: big.NewInt(0),
	Meta:    remAccMeta.ActorMeta(),
}

func NewGeneratorWithSectorsAndUpgradeSchedule(numSectors int, us stmgr.UpgradeSchedule) (*ChainGen, error) {
	j := journal.NilJournal()
	// TODO: we really shouldn't modify a global variable here.
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)

	mr := repo.NewMemory(nil)
	lr, err := mr.Lock(repo.StorageMiner)
	if err != nil {
		return nil, xerrors.Errorf("taking mem-repo lock failed: %w", err)
	}

	ds, err := lr.Datastore(context.TODO(), "/metadata")
	if err != nil {
		return nil, xerrors.Errorf("failed to get metadata datastore: %w", err)
	}

	bs, err := lr.Blockstore(context.TODO(), repo.UniversalBlockstore)
	if err != nil {
		return nil, err
	}

	defer func() {
		if c, ok := bs.(io.Closer); ok {
			if err := c.Close(); err != nil {
				log.Warnf("failed to close blockstore: %s", err)
			}
		}
	}()

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, xerrors.Errorf("getting repo keystore failed: %w", err)
	}

	w, err := wallet.NewWallet(ks)
	if err != nil {
		return nil, xerrors.Errorf("creating memrepo wallet failed: %w", err)
	}

	banker, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate banker key: %w", err)
	}

	receievers := make([]address.Address, msgsPerBlock)
	for r := range receievers {
		receievers[r], err = w.WalletNew(context.Background(), types.KTBLS)
		if err != nil {
			return nil, xerrors.Errorf("failed to generate receiver key: %w", err)
		}
	}

	maddr1 := genesis2.MinerAddress(0)

	m1temp, err := os.MkdirTemp("", "preseal")
	if err != nil {
		return nil, err
	}

	genm1, k1, err := seed.PreSeal(maddr1, abi.RegisteredSealProof_StackedDrg2KiBV1, 0, numSectors, m1temp, []byte("some randomness"), nil, true)
	if err != nil {
		return nil, err
	}

	maddr2 := genesis2.MinerAddress(1)

	m2temp, err := os.MkdirTemp("", "preseal")
	if err != nil {
		return nil, err
	}

	genm2, k2, err := seed.PreSeal(maddr2, abi.RegisteredSealProof_StackedDrg2KiBV1, 0, numSectors, m2temp, []byte("some randomness"), nil, true)
	if err != nil {
		return nil, err
	}

	mk1, err := w.WalletImport(context.Background(), k1)
	if err != nil {
		return nil, err
	}
	mk2, err := w.WalletImport(context.Background(), k2)
	if err != nil {
		return nil, err
	}

	sys := vm.Syscalls(&genFakeVerifier{})

	tpl := genesis.Template{
		NetworkVersion: network.Version0,
		Accounts: []genesis.Actor{
			{
				Type:    genesis.TAccount,
				Balance: types.FromFil(20_000_000),
				Meta:    (&genesis.AccountMeta{Owner: mk1}).ActorMeta(),
			},
			{
				Type:    genesis.TAccount,
				Balance: types.FromFil(20_000_000),
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
		VerifregRootKey:  DefaultVerifregRootkeyActor,
		RemainderAccount: DefaultRemainderAccountActor,
		NetworkName:      uuid.New().String(),
		Timestamp:        uint64(build.Clock.Now().Add(-500 * time.Duration(buildconstants.BlockDelaySecs) * time.Second).Unix()),
	}

	genb, err := genesis2.MakeGenesisBlock(context.TODO(), j, bs, sys, tpl)
	if err != nil {
		return nil, xerrors.Errorf("make genesis block failed: %w", err)
	}

	cs := store.NewChainStore(bs, bs, ds, filcns.Weight, j)

	genfb := &types.FullBlock{Header: genb.Genesis}
	gents := store.NewFullTipSet([]*types.FullBlock{genfb})

	if err := cs.SetGenesis(context.TODO(), genb.Genesis); err != nil {
		return nil, xerrors.Errorf("set genesis failed: %w", err)
	}

	mgen := make(map[address.Address]WinningPoStProver)
	for i := range tpl.Miners {
		mgen[genesis2.MinerAddress(uint64(i))] = &wppProvider{}
	}

	miners := []address.Address{maddr1, maddr2}

	beac := beacon.Schedule{{Start: 0, Beacon: beacon.NewMockBeacon(time.Second)}}
	//beac, err := drand.NewDrandBeacon(tpl.Timestamp, buildconstants.BlockDelaySecs)
	//if err != nil {
	//return nil, xerrors.Errorf("creating drand beacon: %w", err)
	//}

	sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), sys, us, beac, ds, index.DummyMsgIndex)
	if err != nil {
		return nil, xerrors.Errorf("initing stmgr: %w", err)
	}

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

func NewGeneratorWithSectors(numSectors int) (*ChainGen, error) {
	return NewGeneratorWithSectorsAndUpgradeSchedule(numSectors, filcns.DefaultUpgradeSchedule())
}

func NewGeneratorWithUpgradeSchedule(us stmgr.UpgradeSchedule) (*ChainGen, error) {
	return NewGeneratorWithSectorsAndUpgradeSchedule(1, us)
}

func (cg *ChainGen) Blockstore() blockstore.Blockstore {
	return cg.bs
}

func (cg *ChainGen) StateManager() *stmgr.StateManager {
	return cg.sm
}

func (cg *ChainGen) SetStateManager(sm *stmgr.StateManager) {
	cg.sm = sm
}

func (cg *ChainGen) ChainStore() *store.ChainStore {
	return cg.cs
}

func (cg *ChainGen) BeaconSchedule() beacon.Schedule {
	return cg.beacon
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
		pref := link.Cid.Prefix()
		if pref.Codec == cid.FilCommitmentSealed || pref.Codec == cid.FilCommitmentUnsealed {
			continue
		}
		out = append(out, link)
	}

	return out, nil
}

func (cg *ChainGen) nextBlockProof(ctx context.Context, pts *types.TipSet, m address.Address, round abi.ChainEpoch) ([]types.BeaconEntry, *types.ElectionProof, *types.Ticket, error) {
	mc := &mca{w: cg.w, sm: cg.sm, pv: proofsffi.ProofVerifier, bcn: cg.beacon}

	mbi, err := mc.MinerGetBaseInfo(ctx, m, round, pts.Key())
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("get miner base info: %w", err)
	}

	entries := mbi.BeaconEntries
	rbase := mbi.PrevBeaconEntry
	if len(entries) > 0 {
		rbase = entries[len(entries)-1]
	}

	eproof, err := IsRoundWinner(ctx, round, m, rbase, mbi, mc)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("checking round winner failed: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := m.MarshalCBOR(buf); err != nil {
		return nil, nil, nil, xerrors.Errorf("failed to cbor marshal address: %w", err)
	}

	if round > buildconstants.UpgradeSmokeHeight {
		buf.Write(pts.MinTicket().VRFProof)
	}

	ticketRand, err := rand.DrawRandomnessFromBase(rbase.Data, crypto.DomainSeparationTag_TicketProduction, round-buildconstants.TicketRandomnessLookback, buf.Bytes())
	if err != nil {
		return nil, nil, nil, err
	}

	st := pts.ParentState()

	worker, err := stmgr.GetMinerWorkerRaw(ctx, cg.sm, st, m)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("get miner worker: %w", err)
	}

	sf := func(ctx context.Context, a address.Address, i []byte) (*crypto.Signature, error) {
		return cg.w.WalletSign(ctx, a, i, api.MsgMeta{
			Type: api.MTUnknown,
		})
	}

	vrfout, err := ComputeVRF(ctx, sf, worker, ticketRand)
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
	return cg.NextTipSetWithNulls(0)
}

func (cg *ChainGen) NextTipSetWithNulls(nulls abi.ChainEpoch) (*MinedTipSet, error) {
	mts, err := cg.NextTipSetFromMiners(cg.CurTipset.TipSet(), cg.Miners, nulls)
	if err != nil {
		return nil, err
	}

	return mts, nil
}

func (cg *ChainGen) SetWinningPoStProver(m address.Address, wpp WinningPoStProver) {
	cg.eppProvs[m] = wpp
}

func (cg *ChainGen) NextTipSetFromMiners(base *types.TipSet, miners []address.Address, nulls abi.ChainEpoch) (*MinedTipSet, error) {
	ms, err := cg.GetMessages(cg)
	if err != nil {
		return nil, xerrors.Errorf("get random messages: %w", err)
	}

	msgs := make([][]*types.SignedMessage, len(miners))
	for i := range msgs {
		msgs[i] = ms
	}

	fts, err := cg.NextTipSetFromMinersWithMessagesAndNulls(base, miners, msgs, nulls)
	if err != nil {
		return nil, err
	}

	cg.CurTipset = fts

	return &MinedTipSet{
		TipSet:   fts,
		Messages: ms,
	}, nil
}

func (cg *ChainGen) NextTipSetFromMinersWithMessagesAndNulls(base *types.TipSet, miners []address.Address, msgs [][]*types.SignedMessage, nulls abi.ChainEpoch) (*store.FullTipSet, error) {
	ctx := context.TODO()
	var blks []*types.FullBlock

	for round := base.Height() + nulls + 1; len(blks) == 0; round++ {
		for mi, m := range miners {
			bvals, et, ticket, err := cg.nextBlockProof(ctx, base, m, round)
			if err != nil {
				return nil, xerrors.Errorf("next block proof: %w", err)
			}

			if et != nil {
				// TODO: maybe think about passing in more real parameters to this?
				wpost, err := cg.eppProvs[m].ComputeProof(ctx, nil, nil, round, network.Version0)
				if err != nil {
					return nil, err
				}

				fblk, err := cg.makeBlock(base, m, ticket, et, bvals, round, wpost, msgs[mi])
				if err != nil {
					return nil, xerrors.Errorf("making a block for next tipset failed: %w", err)
				}

				blks = append(blks, fblk)
			}
		}
	}

	fts := store.NewFullTipSet(blks)
	if err := cg.cs.PersistTipsets(ctx, []*types.TipSet{fts.TipSet()}); err != nil {
		return nil, xerrors.Errorf("failed to persist tipset: %w", err)
	}

	for _, blk := range blks {
		if err := cg.cs.AddToTipSetTracker(ctx, blk.Header); err != nil {
			return nil, xerrors.Errorf("failed to add to tipset tracker: %w", err)
		}
	}

	if err := cg.cs.RefreshHeaviestTipSet(ctx, fts.TipSet().Height()); err != nil {
		return nil, xerrors.Errorf("failed to put tipset: %w", err)
	}

	cg.CurTipset = fts

	return fts, nil
}

func (cg *ChainGen) makeBlock(parents *types.TipSet, m address.Address, vrfticket *types.Ticket,
	eticket *types.ElectionProof, bvals []types.BeaconEntry, height abi.ChainEpoch,
	wpost []proof7.PoStProof, msgs []*types.SignedMessage) (*types.FullBlock, error) {

	var ts uint64
	if cg.Timestamper != nil {
		ts = cg.Timestamper(parents, height-parents.Height())
	} else {
		ts = parents.MinTimestamp() + uint64(height-parents.Height())*buildconstants.BlockDelaySecs
	}

	fblk, err := filcns.NewFilecoinExpectedConsensus(cg.sm, nil, nil, nil).CreateBlock(context.TODO(), cg.w, &api.BlockTemplate{
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

// ResyncBankerNonce is used for dealing with messages made when
// simulating forks
func (cg *ChainGen) ResyncBankerNonce(ts *types.TipSet) error {
	st, err := cg.sm.ParentState(ts)
	if err != nil {
		return err
	}
	act, err := st.GetActor(cg.banker)
	if err != nil {
		return err
	}
	cg.bankerNonce = act.Nonce

	return nil
}

func (cg *ChainGen) Banker() address.Address {
	return cg.banker
}

func (cg *ChainGen) Wallet() *wallet.LocalWallet {
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

			GasLimit:   100_000_000,
			GasFeeCap:  types.NewInt(0),
			GasPremium: types.NewInt(0),
		}

		sig, err := cg.w.WalletSign(context.TODO(), cg.banker, msg.Cid().Bytes(), api.MsgMeta{
			Type: api.MTUnknown, // testing
		})
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
	StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)

	MinerGetBaseInfo(context.Context, address.Address, abi.ChainEpoch, types.TipSetKey) (*api.MiningBaseInfo, error)

	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
}

type mca struct {
	w   *wallet.LocalWallet
	sm  *stmgr.StateManager
	pv  proofs.Verifier
	bcn beacon.Schedule
}

func (mca mca) StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error) {
	return mca.sm.GetRandomnessFromTickets(ctx, personalization, randEpoch, entropy, tsk)
}

func (mca mca) StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error) {
	return mca.sm.GetRandomnessFromBeacon(ctx, personalization, randEpoch, entropy, tsk)
}

func (mca mca) MinerGetBaseInfo(ctx context.Context, maddr address.Address, epoch abi.ChainEpoch, tsk types.TipSetKey) (*api.MiningBaseInfo, error) {
	return stmgr.MinerGetBaseInfo(ctx, mca.sm, mca.bcn, tsk, epoch, maddr, mca.pv)
}

func (mca mca) WalletSign(ctx context.Context, a address.Address, v []byte) (*crypto.Signature, error) {
	return mca.w.WalletSign(ctx, a, v, api.MsgMeta{
		Type: api.MTUnknown,
	})
}

type WinningPoStProver interface {
	GenerateCandidates(context.Context, abi.PoStRandomness, uint64) ([]uint64, error)
	ComputeProof(context.Context, []proof7.ExtendedSectorInfo, abi.PoStRandomness, abi.ChainEpoch, network.Version) ([]proof7.PoStProof, error)
}

type wppProvider struct{}

func (wpp *wppProvider) GenerateCandidates(ctx context.Context, _ abi.PoStRandomness, _ uint64) ([]uint64, error) {
	return []uint64{0}, nil
}

func (wpp *wppProvider) ComputeProof(context.Context, []proof7.ExtendedSectorInfo, abi.PoStRandomness, abi.ChainEpoch, network.Version) ([]proof7.PoStProof, error) {
	return ValidWpostForTesting, nil
}

func IsRoundWinner(ctx context.Context, round abi.ChainEpoch,
	miner address.Address, brand types.BeaconEntry, mbi *api.MiningBaseInfo, a MiningCheckAPI) (*types.ElectionProof, error) {

	buf := new(bytes.Buffer)
	if err := miner.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to cbor marshal address: %w", err)
	}

	electionRand, err := rand.DrawRandomnessFromBase(brand.Data, crypto.DomainSeparationTag_ElectionProofProduction, round, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to draw randomness: %w", err)
	}

	vrfout, err := ComputeVRF(ctx, a.WalletSign, mbi.WorkerKey, electionRand)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute VRF: %w", err)
	}

	ep := &types.ElectionProof{VRFProof: vrfout}
	j := ep.ComputeWinCount(mbi.MinerPower, mbi.NetworkPower)
	ep.WinCount = j
	if j < 1 {
		return nil, nil
	}

	return ep, nil
}

type SignFunc func(context.Context, address.Address, []byte) (*crypto.Signature, error)

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

var _ proofs.Verifier = (*genFakeVerifier)(nil)

func (m genFakeVerifier) VerifySeal(svi proof7.SealVerifyInfo) (bool, error) {
	return true, nil
}

func (m genFakeVerifier) VerifyAggregateSeals(aggregate proof7.AggregateSealVerifyProofAndInfos) (bool, error) {
	panic("not supported")
}

func (m genFakeVerifier) VerifyReplicaUpdate(update proof7.ReplicaUpdateInfo) (bool, error) {
	panic("not supported")
}

func (m genFakeVerifier) VerifyWinningPoSt(ctx context.Context, info proof7.WinningPoStVerifyInfo) (bool, error) {
	panic("not supported")
}

func (m genFakeVerifier) VerifyWindowPoSt(ctx context.Context, info proof7.WindowPoStVerifyInfo) (bool, error) {
	panic("not supported")
}

func (m genFakeVerifier) GenerateWinningPoStSectorChallenge(ctx context.Context, proof abi.RegisteredPoStProof, id abi.ActorID, randomness abi.PoStRandomness, u uint64) ([]uint64, error) {
	panic("not supported")
}
