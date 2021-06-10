package kit

import (
	"bytes"
	"context"
	"io/ioutil"
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
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	unixfile "github.com/ipfs/go-unixfs/file"
)

type DealHarness struct {
	t      *testing.T
	client *TestFullNode
	miner  *TestMiner
}

// NewDealHarness creates a test harness that contains testing utilities for deals.
func NewDealHarness(t *testing.T, client *TestFullNode, miner *TestMiner) *DealHarness {
	return &DealHarness{
		t:      t,
		client: client,
		miner:  miner,
	}
}

func (dh *DealHarness) MakeOnlineDeal(ctx context.Context, rseed int, fastRet bool, startEpoch abi.ChainEpoch) (deal *cid.Cid, res *api.ImportRes, path string) {
	res, path = dh.client.CreateImportFile(ctx, rseed, 0)

	dh.t.Logf("FILE CID: %s", res.Root)

	deal = dh.StartDeal(ctx, res.Root, fastRet, startEpoch)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)
	dh.WaitDealSealed(ctx, deal, false, false, nil)

	return deal, res, path
}

func (dh *DealHarness) StartDeal(ctx context.Context, fcid cid.Cid, fastRet bool, startEpoch abi.ChainEpoch) *cid.Cid {
	maddr, err := dh.miner.ActorAddress(ctx)
	require.NoError(dh.t, err)

	addr, err := dh.client.WalletDefaultAddress(ctx)
	require.NoError(dh.t, err)

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
	require.NoError(dh.t, err)

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
			dh.t.Log("COMPLETE", di)
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

		dh.t.Logf("Deal %d state: client:%s provider:%s\n", di.DealID, storagemarket.DealStates[di.State], storagemarket.DealStates[minerState])
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
	require.NoError(dh.t, err)

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
					dh.t.Log("COMPLETE", di)
					return
				}
				dh.t.Log("Deal state: ", storagemarket.DealStates[di.State])
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

		dh.miner.FlushSealingBatches(ctx)
	}
}

func (dh *DealHarness) PerformRetrieval(ctx context.Context, deal *cid.Cid, root cid.Cid, carExport bool) (path string) {
	// perform retrieval.
	info, err := dh.client.ClientGetDealInfo(ctx, *deal)
	require.NoError(dh.t, err)

	offers, err := dh.client.ClientFindData(ctx, root, &info.PieceCID)
	require.NoError(dh.t, err)
	require.NotEmpty(dh.t, offers, "no offers")

	tmpfile, err := ioutil.TempFile(dh.t.TempDir(), "ret-car")
	require.NoError(dh.t, err)

	defer tmpfile.Close()

	caddr, err := dh.client.WalletDefaultAddress(ctx)
	require.NoError(dh.t, err)

	ref := &api.FileRef{
		Path:  tmpfile.Name(),
		IsCAR: carExport,
	}

	updates, err := dh.client.ClientRetrieveWithEvents(ctx, offers[0].Order(caddr), ref)
	require.NoError(dh.t, err)

	for update := range updates {
		require.Emptyf(dh.t, update.Err, "retrieval failed: %s", update.Err)
	}

	rdata, err := ioutil.ReadFile(tmpfile.Name())
	require.NoError(dh.t, err)

	if carExport {
		rdata = dh.ExtractFileFromCAR(ctx, rdata)
	}

	return tmpfile.Name()
}

func (dh *DealHarness) ExtractFileFromCAR(ctx context.Context, rdata []byte) []byte {
	bserv := dstest.Bserv()
	ch, err := car.LoadCar(bserv.Blockstore(), bytes.NewReader(rdata))
	require.NoError(dh.t, err)

	b, err := bserv.GetBlock(ctx, ch.Roots[0])
	require.NoError(dh.t, err)

	nd, err := ipld.Decode(b)
	require.NoError(dh.t, err)

	dserv := dag.NewDAGService(bserv)
	fil, err := unixfile.NewUnixfsFile(ctx, dserv, nd)
	require.NoError(dh.t, err)

	tmpfile, err := ioutil.TempFile(dh.t.TempDir(), "file-in-car")
	require.NoError(dh.t, err)

	defer tmpfile.Close()

	err = files.WriteTo(fil, tmpfile.Name())
	require.NoError(dh.t, err)

	rdata, err = ioutil.ReadFile(tmpfile.Name())
	require.NoError(dh.t, err)

	return rdata
}

func ConnectAndStartMining(t *testing.T, blocktime time.Duration, miner *TestMiner, clients ...api.FullNode) *BlockMiner {
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
	t.Cleanup(blockMiner.Stop)
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
