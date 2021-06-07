package kit

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipld/go-car"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/node/impl"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	unixfile "github.com/ipfs/go-unixfs/file"
)

type DealHarness struct {
	t      *testing.T
	client api.FullNode
	miner  TestMiner
}

// NewDealHarness creates a test harness that contains testing utilities for deals.
func NewDealHarness(t *testing.T, client api.FullNode, miner TestMiner) *DealHarness {
	return &DealHarness{
		t:      t,
		client: client,
		miner:  miner,
	}
}

func (dh *DealHarness) MakeFullDeal(ctx context.Context, rseed int, carExport, fastRet bool, startEpoch abi.ChainEpoch) {
	res, data, err := CreateImportFile(ctx, dh.client, rseed, 0)
	if err != nil {
		dh.t.Fatal(err)
	}

	fcid := res.Root
	fmt.Println("FILE CID: ", fcid)

	deal := dh.StartDeal(ctx, fcid, fastRet, startEpoch)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)
	dh.WaitDealSealed(ctx, deal, false, false, nil)

	// Retrieval
	info, err := dh.client.ClientGetDealInfo(ctx, *deal)
	require.NoError(dh.t, err)

	dh.TestRetrieval(ctx, fcid, &info.PieceCID, carExport, data)
}

func (dh *DealHarness) StartDeal(ctx context.Context, fcid cid.Cid, fastRet bool, startEpoch abi.ChainEpoch) *cid.Cid {
	maddr, err := dh.miner.ActorAddress(ctx)
	if err != nil {
		dh.t.Fatal(err)
	}

	addr, err := dh.client.WalletDefaultAddress(ctx)
	if err != nil {
		dh.t.Fatal(err)
	}
	deal, err := dh.client.ClientStartDeal(ctx, &api.StartDealParams{
		Data: &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         fcid,
		},
		Wallet:            addr,
		Miner:             maddr,
		EpochPrice:        types.NewInt(1000000),
		DealStartEpoch:    startEpoch,
		MinBlocksDuration: uint64(build.MinDealDuration),
		FastRetrieval:     fastRet,
	})
	if err != nil {
		dh.t.Fatalf("%+v", err)
	}
	return deal
}

func (dh *DealHarness) WaitDealSealed(ctx context.Context, deal *cid.Cid, noseal, noSealStart bool, cb func()) {
loop:
	for {
		di, err := dh.client.ClientGetDealInfo(ctx, *deal)
		require.NoError(dh.t, err)

		switch di.State {
		case storagemarket.StorageDealAwaitingPreCommit, storagemarket.StorageDealSealing:
			if noseal {
				return
			}
			if !noSealStart {
				dh.StartSealingWaiting(ctx)
			}
		case storagemarket.StorageDealProposalRejected:
			dh.t.Fatal("deal rejected")
		case storagemarket.StorageDealFailing:
			dh.t.Fatal("deal failed")
		case storagemarket.StorageDealError:
			dh.t.Fatal("deal errored", di.Message)
		case storagemarket.StorageDealActive:
			fmt.Println("COMPLETE", di)
			break loop
		}

		mds, err := dh.miner.MarketListIncompleteDeals(ctx)
		require.NoError(dh.t, err)

		var minerState storagemarket.StorageDealStatus
		for _, md := range mds {
			if md.DealID == di.DealID {
				minerState = md.State
				break
			}
		}

		fmt.Printf("Deal %d state: client:%s provider:%s\n", di.DealID, storagemarket.DealStates[di.State], storagemarket.DealStates[minerState])
		time.Sleep(time.Second / 2)
		if cb != nil {
			cb()
		}
	}
}

func (dh *DealHarness) WaitDealPublished(ctx context.Context, deal *cid.Cid) {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	updates, err := dh.miner.MarketGetDealUpdates(subCtx)
	if err != nil {
		dh.t.Fatal(err)
	}
	for {
		select {
		case <-ctx.Done():
			dh.t.Fatal("context timeout")
		case di := <-updates:
			if deal.Equals(di.ProposalCid) {
				switch di.State {
				case storagemarket.StorageDealProposalRejected:
					dh.t.Fatal("deal rejected")
				case storagemarket.StorageDealFailing:
					dh.t.Fatal("deal failed")
				case storagemarket.StorageDealError:
					dh.t.Fatal("deal errored", di.Message)
				case storagemarket.StorageDealFinalizing, storagemarket.StorageDealAwaitingPreCommit, storagemarket.StorageDealSealing, storagemarket.StorageDealActive:
					fmt.Println("COMPLETE", di)
					return
				}
				fmt.Println("Deal state: ", storagemarket.DealStates[di.State])
			}
		}
	}
}

func (dh *DealHarness) StartSealingWaiting(ctx context.Context) {
	snums, err := dh.miner.SectorsList(ctx)
	require.NoError(dh.t, err)

	for _, snum := range snums {
		si, err := dh.miner.SectorsStatus(ctx, snum, false)
		require.NoError(dh.t, err)

		dh.t.Logf("Sector state: %s", si.State)
		if si.State == api.SectorState(sealing.WaitDeals) {
			require.NoError(dh.t, dh.miner.SectorStartSealing(ctx, snum))
		}
	}
}

func (dh *DealHarness) TestRetrieval(ctx context.Context, fcid cid.Cid, piece *cid.Cid, carExport bool, data []byte) {
	offers, err := dh.client.ClientFindData(ctx, fcid, piece)
	if err != nil {
		dh.t.Fatal(err)
	}

	if len(offers) < 1 {
		dh.t.Fatal("no offers")
	}

	rpath, err := ioutil.TempDir("", "lotus-retrieve-test-")
	if err != nil {
		dh.t.Fatal(err)
	}
	defer os.RemoveAll(rpath) //nolint:errcheck

	caddr, err := dh.client.WalletDefaultAddress(ctx)
	if err != nil {
		dh.t.Fatal(err)
	}

	ref := &api.FileRef{
		Path:  filepath.Join(rpath, "ret"),
		IsCAR: carExport,
	}
	updates, err := dh.client.ClientRetrieveWithEvents(ctx, offers[0].Order(caddr), ref)
	if err != nil {
		dh.t.Fatal(err)
	}
	for update := range updates {
		if update.Err != "" {
			dh.t.Fatalf("retrieval failed: %s", update.Err)
		}
	}

	rdata, err := ioutil.ReadFile(filepath.Join(rpath, "ret"))
	if err != nil {
		dh.t.Fatal(err)
	}

	if carExport {
		rdata = dh.ExtractCarData(ctx, rdata, rpath)
	}

	if !bytes.Equal(rdata, data) {
		dh.t.Fatal("wrong data retrieved")
	}
}

func (dh *DealHarness) ExtractCarData(ctx context.Context, rdata []byte, rpath string) []byte {
	bserv := dstest.Bserv()
	ch, err := car.LoadCar(bserv.Blockstore(), bytes.NewReader(rdata))
	if err != nil {
		dh.t.Fatal(err)
	}
	b, err := bserv.GetBlock(ctx, ch.Roots[0])
	if err != nil {
		dh.t.Fatal(err)
	}
	nd, err := ipld.Decode(b)
	if err != nil {
		dh.t.Fatal(err)
	}
	dserv := dag.NewDAGService(bserv)
	fil, err := unixfile.NewUnixfsFile(ctx, dserv, nd)
	if err != nil {
		dh.t.Fatal(err)
	}
	outPath := filepath.Join(rpath, "retLoadedCAR")
	if err := files.WriteTo(fil, outPath); err != nil {
		dh.t.Fatal(err)
	}
	rdata, err = ioutil.ReadFile(outPath)
	if err != nil {
		dh.t.Fatal(err)
	}
	return rdata
}

type DealsScaffold struct {
	Ctx        context.Context
	Client     *impl.FullNodeAPI
	Miner      TestMiner
	BlockMiner *BlockMiner
}

func ConnectAndStartMining(t *testing.T, blocktime time.Duration, miner TestMiner, clients ...api.FullNode) *BlockMiner {
	ctx := context.Background()

	for _, c := range clients {
		addrinfo, err := c.NetAddrsListen(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := miner.NetConnect(ctx, addrinfo); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(time.Second)

	blockMiner := NewBlockMiner(t, miner)
	blockMiner.MineBlocks(ctx, blocktime)

	return blockMiner
}

type TestDealState int

const (
	TestDealStateFailed     = TestDealState(-1)
	TestDealStateInProgress = TestDealState(0)
	TestDealStateComplete   = TestDealState(1)
)

// CategorizeDealState categorizes deal states into one of three states:
// Complete, InProgress, Failed.
func CategorizeDealState(dealStatus string) TestDealState {
	switch dealStatus {
	case "StorageDealFailing", "StorageDealError":
		return TestDealStateFailed
	case "StorageDealStaged", "StorageDealAwaitingPreCommit", "StorageDealSealing", "StorageDealActive", "StorageDealExpired", "StorageDealSlashed":
		return TestDealStateComplete
	}
	return TestDealStateInProgress
}
