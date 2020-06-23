package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"time"

	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/peer"
	crypto "github.com/libp2p/go-libp2p-crypto"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-storedcounter"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	modtest "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
)

func init() {
	power.ConsensusMinerMinPower = big.NewInt(2048)
	saminer.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)
	build.DisableBuiltinAssets = true
}

var testcases = map[string]interface{}{
	"lotus-network": lotusNetwork(),
}

type GenesisMessage struct {
	GenBuf []byte
}

func main() {
	run.InvokeMap(testcases)
}

func setupLotusNode(genesis []byte) (api.FullNode, error) {
	ctx := context.Background()
	mn := mocknet.New(ctx)

	genOpt := node.Override(new(modules.Genesis), modules.LoadGenesis(genesis))

	var fullNode api.FullNode
	_, err := node.New(ctx,
		node.FullAPI(&fullNode),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.MockHost(mn),
		node.Test(),

		genOpt,
	)
	if err != nil {
		return nil, err
	}

	return fullNode, nil
}

func testStorageNode(ctx context.Context, waddr address.Address, act address.Address, tnd api.FullNode, opts node.Option, numPreSeals int) (api.StorageMiner, error) {
	r := repo.NewMemory(nil)

	pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	lr, err := r.Lock(repo.StorageMiner)
	if err != nil {
		return nil, err
	}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, err
	}

	kbytes, err := pk.Bytes()
	if err != nil {
		return nil, err
	}

	err = ks.Put("libp2p-host", types.KeyInfo{
		Type:       "libp2p-host",
		PrivateKey: kbytes,
	})
	if err != nil {
		return nil, err
	}

	ds, err := lr.Datastore("/metadata")
	if err != nil {
		return nil, err
	}

	err = ds.Put(datastore.NewKey("miner-address"), act.Bytes())
	if err != nil {
		return nil, err
	}

	nic := storedcounter.New(ds, datastore.NewKey("/storage/nextid"))
	for i := 0; i < numPreSeals; i++ {
		nic.Next()
	}
	nic.Next()

	if err := lr.Close(); err != nil {
		return nil, err
	}

	peerid, err := peer.IDFromPrivateKey(pk)
	if err != nil {
		return nil, err
	}

	enc, err := actors.SerializeParams(&saminer.ChangePeerIDParams{NewID: abi.PeerID(peerid)})
	if err != nil {
		return nil, err
	}

	msg := &types.Message{
		To:       act,
		From:     waddr,
		Method:   builtin.MethodsMiner.ChangePeerID,
		Params:   enc,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: 1000000,
	}

	if _, err := tnd.MpoolPushMessage(ctx, msg); err != nil {
		return nil, err
	}

	// start node
	var minerapi api.StorageMiner

	mineBlock := make(chan func(bool))
	// TODO: use stop
	_, err = node.New(ctx,
		node.StorageMiner(&minerapi),
		node.Online(),
		node.Repo(r),
		node.Test(),

		node.Override(new(api.FullNode), tnd),
		node.Override(new(*miner.Miner), miner.NewTestMiner(mineBlock, act)),

		opts,
	)
	if err != nil {
		return nil, err
	}

	return minerapi, nil
}

func setupStorageNode(runenv *runtime.RunEnv, fnode api.FullNode, key *wallet.Key, maddr, worker address.Address, preSealDir string, numPreSeals int) (api.StorageMiner, error) {
	ctx := context.TODO()
	runenv.RecordMessage("Wallet Import")
	if _, err := fnode.WalletImport(ctx, &key.KeyInfo); err != nil {
		return nil, err
	}
	runenv.RecordMessage("Wallet Set Defaults")
	if err := fnode.WalletSetDefault(ctx, key.Address); err != nil {
		return nil, err
	}

	runenv.RecordMessage("test Storage Node")
	storageNode, err := testStorageNode(ctx, worker, maddr, fnode, node.Options(), numPreSeals)
	if err != nil {
		return nil, err
	}

	if preSealDir != "" {
		runenv.RecordMessage("Storage Add Local")
		if err := storageNode.StorageAddLocal(ctx, preSealDir); err != nil {
			return nil, err
		}
	}
	return storageNode, nil
}

type PreSealInfo struct {
	Dir      string
	GenAct   genesis.Actor
	GenMiner genesis.Miner
	WKey     *wallet.Key
}

const nGenesisPreseals = 4

func runPreSeal(runenv *runtime.RunEnv, maddr address.Address, minerPid peer.ID) (*PreSealInfo, error) {
	tdir, err := ioutil.TempDir("", "preseal-memgen")
	if err != nil {
		return nil, err
	}

	genm, k, err := seed.PreSeal(maddr, abi.RegisteredSealProof_StackedDrg2KiBV1, 0, nGenesisPreseals, tdir, []byte("make genesis mem random"), nil)
	if err != nil {
		return nil, err
	}
	genm.PeerId = minerPid

	wk, err := wallet.NewKey(*k)
	if err != nil {
		return nil, nil
	}

	genAct := genesis.Actor{
		Type:    genesis.TAccount,
		Balance: big.NewInt(5000000000000000000),
		Meta:    (&genesis.AccountMeta{Owner: wk.Address}).ActorMeta(),
	}

	return &PreSealInfo{
		Dir:      tdir,
		GenAct:   genAct,
		GenMiner: *genm,
		WKey:     wk,
	}, nil
}

func lotusNetwork() run.InitializedTestCaseFn {
	return func(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		var (
			client = initCtx.SyncClient
			seq    = initCtx.GlobalSeq
		)

		minerCount := runenv.IntParam("miner-count")
		if minerCount > runenv.TestInstanceCount {
			return errors.New("cannot have more miners than nodes")
		}

		var maddr address.Address
		var minerPid peer.ID
		var mpsi *PreSealInfo

		preSealTopic := sync.NewTopic("preseals", &PreSealInfo{})
		if seq <= int64(minerCount) { // sequence numbers start at 1
			runenv.RecordMessage("Running preseal (seq = %d)", seq)

			pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
			if err != nil {
				return err
			}

			mpid, err := peer.IDFromPrivateKey(pk)
			if err != nil {
				return err
			}
			minerPid = mpid

			ma, err := address.NewIDAddress(uint64(1000 + seq - 1))
			if err != nil {
				return err
			}
			maddr = ma

			psi, err := runPreSeal(runenv, maddr, minerPid)
			if err != nil {
				return err
			}

			mpsi = psi
			client.Publish(ctx, preSealTopic, psi)
		}

		//time.Sleep(300 * time.Second)

		genesisTopic := sync.NewTopic("genesis", &GenesisMessage{})

		var genesisBytes []byte

		if seq == 1 {
			var genaccs []genesis.Actor
			var genms []genesis.Miner

			var preSeals []*PreSealInfo

			psch := make(chan *PreSealInfo)
			client.MustSubscribe(ctx, preSealTopic, psch)
			for i := 0; i < minerCount; i++ {
				psi := <-psch
				preSeals = append(preSeals, psi)
				genms = append(genms, psi.GenMiner)
				genaccs = append(genaccs, psi.GenAct)
			}

			runenv.RecordMessage("have %d genesis miners", len(preSeals))

			templ := &genesis.Template{
				Accounts:  genaccs,
				Miners:    genms,
				Timestamp: uint64(time.Now().Unix() - 10000), // some time sufficiently far in the past
			}

			runenv.RecordMessage("genminer: %s %s", genms[0].Owner, genms[0].Worker)

			runenv.RecordMessage("making a genesis file: %d %d", len(templ.Accounts), len(templ.Miners))

			bs := blockstore.NewBlockstore(datastore.NewMapDatastore())
			bs = blockstore.NewIdStore(bs)
			var genbuf bytes.Buffer
			_, err := modtest.MakeGenesisMem(&genbuf, *templ)(bs, vm.Syscalls(&genFakeVerifier{}))()
			if err != nil {
				runenv.RecordMessage("genesis file failure: %v", err)
				return xerrors.Errorf("failed to make genesis file: %w", err)
			}

			runenv.RecordMessage("now broadcasting genesis file (len = %d)", genbuf.Len())

			genesisBytes = genbuf.Bytes()
			client.MustPublish(ctx, genesisTopic, &GenesisMessage{
				GenBuf: genbuf.Bytes(),
			})
		} else {
			gench := make(chan *GenesisMessage)
			client.MustSubscribe(ctx, genesisTopic, gench)

			genm := <-gench
			genesisBytes = genm.GenBuf
		}

		runenv.RecordMessage("about to set up lotus node (len = %d)", len(genesisBytes))
		lnode, err := setupLotusNode(genesisBytes)
		if err != nil {
			return err
		}

		id, err := lnode.ID(ctx)
		if err != nil {
			return err
		}

		runenv.RecordMessage("Lotus node ID is: %s", id)

		gents, err := lnode.ChainGetGenesis(ctx)
		if err != nil {
			return err
		}

		runenv.RecordMessage("Genesis cid: %s", gents.Key())

		withMiner := seq < int64(minerCount)

		if withMiner {
			runenv.RecordMessage("Setup storage node")
			sminer, err := setupStorageNode(runenv, lnode, mpsi.WKey, maddr, mpsi.WKey.Address, mpsi.Dir, nGenesisPreseals)
			if err != nil {
				return xerrors.Errorf("failed to set up storage miner: %w", err)
			}

			fmt.Println(sminer)
		}

		return nil
	}
}

func sameAddrs(a, b []net.Addr) bool {
	if len(a) != len(b) {
		return false
	}
	aset := make(map[string]bool, len(a))
	for _, addr := range a {
		aset[addr.String()] = true
	}
	for _, addr := range b {
		if !aset[addr.String()] {
			return false
		}
	}
	return true
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

func (m genFakeVerifier) GenerateWinningPoStSectorChallenge(ctx context.Context, proof abi.RegisteredPoStProof, id abi.ActorID, randomness abi.PoStRandomness, u uint64) ([]uint64, error) {
	panic("not supported")
}
