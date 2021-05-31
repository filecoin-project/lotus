package test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipld/go-car"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	unixfile "github.com/ipfs/go-unixfs/file"
)

func TestDealFlow(t *testing.T, b APIBuilder, blocktime time.Duration, carExport, fastRet bool, startEpoch abi.ChainEpoch) {
	s := setupOneClientOneMiner(t, b, blocktime)
	defer s.blockMiner.Stop()

	MakeDeal(t, s.ctx, 6, s.client, s.miner, carExport, fastRet, startEpoch)
}

func TestDoubleDealFlow(t *testing.T, b APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	s := setupOneClientOneMiner(t, b, blocktime)
	defer s.blockMiner.Stop()

	MakeDeal(t, s.ctx, 6, s.client, s.miner, false, false, startEpoch)
	MakeDeal(t, s.ctx, 7, s.client, s.miner, false, false, startEpoch)
}

func MakeDeal(t *testing.T, ctx context.Context, rseed int, client api.FullNode, miner TestStorageNode, carExport, fastRet bool, startEpoch abi.ChainEpoch) {
	res, data, err := CreateClientFile(ctx, client, rseed, 0)
	if err != nil {
		t.Fatal(err)
	}

	fcid := res.Root
	fmt.Println("FILE CID: ", fcid)

	deal := startDeal(t, ctx, miner, client, fcid, fastRet, startEpoch)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)
	waitDealSealed(t, ctx, miner, client, deal, false, false, nil)

	// Retrieval
	info, err := client.ClientGetDealInfo(ctx, *deal)
	require.NoError(t, err)

	testRetrieval(t, ctx, client, fcid, &info.PieceCID, carExport, data)
}

func CreateClientFile(ctx context.Context, client api.FullNode, rseed, size int) (*api.ImportRes, []byte, error) {
	if size == 0 {
		size = 1600
	}
	data := make([]byte, size)
	rand.New(rand.NewSource(int64(rseed))).Read(data)

	dir, err := ioutil.TempDir(os.TempDir(), "test-make-deal-")
	if err != nil {
		return nil, nil, err
	}

	path := filepath.Join(dir, "sourcefile.dat")
	err = ioutil.WriteFile(path, data, 0644)
	if err != nil {
		return nil, nil, err
	}

	res, err := client.ClientImport(ctx, api.FileRef{Path: path})
	if err != nil {
		return nil, nil, err
	}
	return res, data, nil
}

func TestPublishDealsBatching(t *testing.T, b APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	publishPeriod := 10 * time.Second
	maxDealsPerMsg := uint64(2)

	// Set max deals per publish deals message to 2
	minerDef := []StorageMiner{{
		Full: 0,
		Opts: node.Override(
			new(*storageadapter.DealPublisher),
			storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
				Period:         publishPeriod,
				MaxDealsPerMsg: maxDealsPerMsg,
			})),
		Preseal: PresealGenesis,
	}}

	// Create a connect client and miner node
	n, sn := b(t, OneFull, minerDef)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]
	s := connectAndStartMining(t, b, blocktime, client, miner)
	defer s.blockMiner.Stop()

	// Starts a deal and waits until it's published
	runDealTillPublish := func(rseed int) {
		res, _, err := CreateClientFile(s.ctx, s.client, rseed, 0)
		require.NoError(t, err)

		upds, err := client.ClientGetDealUpdates(s.ctx)
		require.NoError(t, err)

		startDeal(t, s.ctx, s.miner, s.client, res.Root, false, startEpoch)

		// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
		time.Sleep(time.Second)

		done := make(chan struct{})
		go func() {
			for upd := range upds {
				if upd.DataRef.Root == res.Root && upd.State == storagemarket.StorageDealAwaitingPreCommit {
					done <- struct{}{}
				}
			}
		}()
		<-done
	}

	// Run three deals in parallel
	done := make(chan struct{}, maxDealsPerMsg+1)
	for rseed := 1; rseed <= 3; rseed++ {
		rseed := rseed
		go func() {
			runDealTillPublish(rseed)
			done <- struct{}{}
		}()
	}

	// Wait for two of the deals to be published
	for i := 0; i < int(maxDealsPerMsg); i++ {
		<-done
	}

	// Expect a single PublishStorageDeals message that includes the first two deals
	msgCids, err := s.client.StateListMessages(s.ctx, &api.MessageMatch{To: market.Address}, types.EmptyTSK, 1)
	require.NoError(t, err)
	count := 0
	for _, msgCid := range msgCids {
		msg, err := s.client.ChainGetMessage(s.ctx, msgCid)
		require.NoError(t, err)

		if msg.Method == market.Methods.PublishStorageDeals {
			count++
			var pubDealsParams market2.PublishStorageDealsParams
			err = pubDealsParams.UnmarshalCBOR(bytes.NewReader(msg.Params))
			require.NoError(t, err)
			require.Len(t, pubDealsParams.Deals, int(maxDealsPerMsg))
		}
	}
	require.Equal(t, 1, count)

	// The third deal should be published once the publish period expires.
	// Allow a little padding as it takes a moment for the state change to
	// be noticed by the client.
	padding := 10 * time.Second
	select {
	case <-time.After(publishPeriod + padding):
		require.Fail(t, "Expected 3rd deal to be published once publish period elapsed")
	case <-done: // Success
	}
}

func TestBatchDealInput(t *testing.T, b APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	run := func(piece, deals, expectSectors int) func(t *testing.T) {
		return func(t *testing.T) {
			publishPeriod := 10 * time.Second
			maxDealsPerMsg := uint64(deals)

			// Set max deals per publish deals message to maxDealsPerMsg
			minerDef := []StorageMiner{{
				Full: 0,
				Opts: node.Options(
					node.Override(
						new(*storageadapter.DealPublisher),
						storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
							Period:         publishPeriod,
							MaxDealsPerMsg: maxDealsPerMsg,
						})),
					node.Override(new(dtypes.GetSealingConfigFunc), func() (dtypes.GetSealingConfigFunc, error) {
						return func() (sealiface.Config, error) {
							return sealiface.Config{
								MaxWaitDealsSectors:       2,
								MaxSealingSectors:         1,
								MaxSealingSectorsForDeals: 3,
								AlwaysKeepUnsealedCopy:    true,
								WaitDealsDelay:            time.Hour,
							}, nil
						}, nil
					}),
				),
				Preseal: PresealGenesis,
			}}

			// Create a connect client and miner node
			n, sn := b(t, OneFull, minerDef)
			client := n[0].FullNode.(*impl.FullNodeAPI)
			miner := sn[0]
			s := connectAndStartMining(t, b, blocktime, client, miner)
			defer s.blockMiner.Stop()

			err := miner.MarketSetAsk(s.ctx, big.Zero(), big.Zero(), 200, 128, 32<<30)
			require.NoError(t, err)

			checkNoPadding := func() {
				sl, err := sn[0].SectorsList(s.ctx)
				require.NoError(t, err)

				sort.Slice(sl, func(i, j int) bool {
					return sl[i] < sl[j]
				})

				for _, snum := range sl {
					si, err := sn[0].SectorsStatus(s.ctx, snum, false)
					require.NoError(t, err)

					// fmt.Printf("S %d: %+v %s\n", snum, si.Deals, si.State)

					for _, deal := range si.Deals {
						if deal == 0 {
							fmt.Printf("sector %d had a padding piece!\n", snum)
						}
					}
				}
			}

			// Starts a deal and waits until it's published
			runDealTillSeal := func(rseed int) {
				res, _, err := CreateClientFile(s.ctx, s.client, rseed, piece)
				require.NoError(t, err)

				dc := startDeal(t, s.ctx, s.miner, s.client, res.Root, false, startEpoch)
				waitDealSealed(t, s.ctx, s.miner, s.client, dc, false, true, checkNoPadding)
			}

			// Run maxDealsPerMsg deals in parallel
			done := make(chan struct{}, maxDealsPerMsg)
			for rseed := 0; rseed < int(maxDealsPerMsg); rseed++ {
				rseed := rseed
				go func() {
					runDealTillSeal(rseed)
					done <- struct{}{}
				}()
			}

			// Wait for maxDealsPerMsg of the deals to be published
			for i := 0; i < int(maxDealsPerMsg); i++ {
				<-done
			}

			checkNoPadding()

			sl, err := sn[0].SectorsList(s.ctx)
			require.NoError(t, err)
			require.Equal(t, len(sl), expectSectors)
		}
	}

	t.Run("4-p1600B", run(1600, 4, 4))
	t.Run("4-p513B", run(513, 4, 2))
	if !testing.Short() {
		t.Run("32-p257B", run(257, 32, 8))
		t.Run("32-p10B", run(10, 32, 2))

		// fixme: this appears to break data-transfer / markets in some really creative ways
		//t.Run("128-p10B", run(10, 128, 8))
	}
}

func TestFastRetrievalDealFlow(t *testing.T, b APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	s := setupOneClientOneMiner(t, b, blocktime)
	defer s.blockMiner.Stop()

	data := make([]byte, 1600)
	rand.New(rand.NewSource(int64(8))).Read(data)

	r := bytes.NewReader(data)
	fcid, err := s.client.ClientImportLocal(s.ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FILE CID: ", fcid)

	deal := startDeal(t, s.ctx, s.miner, s.client, fcid, true, startEpoch)

	waitDealPublished(t, s.ctx, s.miner, deal)
	fmt.Println("deal published, retrieving")
	// Retrieval
	info, err := s.client.ClientGetDealInfo(s.ctx, *deal)
	require.NoError(t, err)

	testRetrieval(t, s.ctx, s.client, fcid, &info.PieceCID, false, data)
}

func TestSecondDealRetrieval(t *testing.T, b APIBuilder, blocktime time.Duration) {
	s := setupOneClientOneMiner(t, b, blocktime)
	defer s.blockMiner.Stop()

	{
		data1 := make([]byte, 800)
		rand.New(rand.NewSource(int64(3))).Read(data1)
		r := bytes.NewReader(data1)

		fcid1, err := s.client.ClientImportLocal(s.ctx, r)
		if err != nil {
			t.Fatal(err)
		}

		data2 := make([]byte, 800)
		rand.New(rand.NewSource(int64(9))).Read(data2)
		r2 := bytes.NewReader(data2)

		fcid2, err := s.client.ClientImportLocal(s.ctx, r2)
		if err != nil {
			t.Fatal(err)
		}

		deal1 := startDeal(t, s.ctx, s.miner, s.client, fcid1, true, 0)

		// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
		time.Sleep(time.Second)
		waitDealSealed(t, s.ctx, s.miner, s.client, deal1, true, false, nil)

		deal2 := startDeal(t, s.ctx, s.miner, s.client, fcid2, true, 0)

		time.Sleep(time.Second)
		waitDealSealed(t, s.ctx, s.miner, s.client, deal2, false, false, nil)

		// Retrieval
		info, err := s.client.ClientGetDealInfo(s.ctx, *deal2)
		require.NoError(t, err)

		rf, _ := s.miner.SectorsRefs(s.ctx)
		fmt.Printf("refs: %+v\n", rf)

		testRetrieval(t, s.ctx, s.client, fcid2, &info.PieceCID, false, data2)
	}
}

func TestZeroPricePerByteRetrievalDealFlow(t *testing.T, b APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	s := setupOneClientOneMiner(t, b, blocktime)
	defer s.blockMiner.Stop()

	// Set price-per-byte to zero
	ask, err := s.miner.MarketGetRetrievalAsk(s.ctx)
	require.NoError(t, err)

	ask.PricePerByte = abi.NewTokenAmount(0)
	err = s.miner.MarketSetRetrievalAsk(s.ctx, ask)
	require.NoError(t, err)

	MakeDeal(t, s.ctx, 6, s.client, s.miner, false, false, startEpoch)
}

func startDeal(t *testing.T, ctx context.Context, miner TestStorageNode, client api.FullNode, fcid cid.Cid, fastRet bool, startEpoch abi.ChainEpoch) *cid.Cid {
	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deal, err := client.ClientStartDeal(ctx, &api.StartDealParams{
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
		t.Fatalf("%+v", err)
	}
	return deal
}

func waitDealSealed(t *testing.T, ctx context.Context, miner TestStorageNode, client api.FullNode, deal *cid.Cid, noseal, noSealStart bool, cb func()) {
loop:
	for {
		di, err := client.ClientGetDealInfo(ctx, *deal)
		if err != nil {
			t.Fatal(err)
		}
		switch di.State {
		case storagemarket.StorageDealAwaitingPreCommit, storagemarket.StorageDealSealing:
			if noseal {
				return
			}
			if !noSealStart {
				startSealingWaiting(t, ctx, miner)
			}
		case storagemarket.StorageDealProposalRejected:
			t.Fatal("deal rejected")
		case storagemarket.StorageDealFailing:
			t.Fatal("deal failed")
		case storagemarket.StorageDealError:
			t.Fatal("deal errored", di.Message)
		case storagemarket.StorageDealActive:
			fmt.Println("COMPLETE", di)
			break loop
		}

		mds, err := miner.MarketListIncompleteDeals(ctx)
		if err != nil {
			t.Fatal(err)
		}

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

func waitDealPublished(t *testing.T, ctx context.Context, miner TestStorageNode, deal *cid.Cid) {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	updates, err := miner.MarketGetDealUpdates(subCtx)
	if err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatal("context timeout")
		case di := <-updates:
			if deal.Equals(di.ProposalCid) {
				switch di.State {
				case storagemarket.StorageDealProposalRejected:
					t.Fatal("deal rejected")
				case storagemarket.StorageDealFailing:
					t.Fatal("deal failed")
				case storagemarket.StorageDealError:
					t.Fatal("deal errored", di.Message)
				case storagemarket.StorageDealFinalizing, storagemarket.StorageDealAwaitingPreCommit, storagemarket.StorageDealSealing, storagemarket.StorageDealActive:
					fmt.Println("COMPLETE", di)
					return
				}
				fmt.Println("Deal state: ", storagemarket.DealStates[di.State])
			}
		}
	}
}

func startSealingWaiting(t *testing.T, ctx context.Context, miner TestStorageNode) {
	snums, err := miner.SectorsList(ctx)
	require.NoError(t, err)

	for _, snum := range snums {
		si, err := miner.SectorsStatus(ctx, snum, false)
		require.NoError(t, err)

		t.Logf("Sector %d state: %s", snum, si.State)
		if si.State == api.SectorState(sealing.WaitDeals) {
			require.NoError(t, miner.SectorStartSealing(ctx, snum))
		}
	}

	flushSealingBatches(t, ctx, miner)
}

func testRetrieval(t *testing.T, ctx context.Context, client api.FullNode, fcid cid.Cid, piece *cid.Cid, carExport bool, data []byte) {
	offers, err := client.ClientFindData(ctx, fcid, piece)
	if err != nil {
		t.Fatal(err)
	}

	if len(offers) < 1 {
		t.Fatal("no offers")
	}

	rpath, err := ioutil.TempDir("", "lotus-retrieve-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(rpath) //nolint:errcheck

	caddr, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	ref := &api.FileRef{
		Path:  filepath.Join(rpath, "ret"),
		IsCAR: carExport,
	}
	updates, err := client.ClientRetrieveWithEvents(ctx, offers[0].Order(caddr), ref)
	if err != nil {
		t.Fatal(err)
	}
	for update := range updates {
		if update.Err != "" {
			t.Fatalf("retrieval failed: %s", update.Err)
		}
	}

	rdata, err := ioutil.ReadFile(filepath.Join(rpath, "ret"))
	if err != nil {
		t.Fatal(err)
	}

	if carExport {
		rdata = extractCarData(t, ctx, rdata, rpath)
	}

	if !bytes.Equal(rdata, data) {
		t.Fatal("wrong data retrieved")
	}
}

func extractCarData(t *testing.T, ctx context.Context, rdata []byte, rpath string) []byte {
	bserv := dstest.Bserv()
	ch, err := car.LoadCar(bserv.Blockstore(), bytes.NewReader(rdata))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bserv.GetBlock(ctx, ch.Roots[0])
	if err != nil {
		t.Fatal(err)
	}
	nd, err := ipld.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	dserv := dag.NewDAGService(bserv)
	fil, err := unixfile.NewUnixfsFile(ctx, dserv, nd)
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(rpath, "retLoadedCAR")
	if err := files.WriteTo(fil, outPath); err != nil {
		t.Fatal(err)
	}
	rdata, err = ioutil.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	return rdata
}

type dealsScaffold struct {
	ctx        context.Context
	client     *impl.FullNodeAPI
	miner      TestStorageNode
	blockMiner *BlockMiner
}

func setupOneClientOneMiner(t *testing.T, b APIBuilder, blocktime time.Duration) *dealsScaffold {
	n, sn := b(t, OneFull, OneMiner)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]
	return connectAndStartMining(t, b, blocktime, client, miner)
}

func connectAndStartMining(t *testing.T, b APIBuilder, blocktime time.Duration, client *impl.FullNodeAPI, miner TestStorageNode) *dealsScaffold {
	ctx := context.Background()
	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)

	blockMiner := NewBlockMiner(ctx, t, miner, blocktime)
	blockMiner.MineBlocks()

	return &dealsScaffold{
		ctx:        ctx,
		client:     client,
		miner:      miner,
		blockMiner: blockMiner,
	}
}
