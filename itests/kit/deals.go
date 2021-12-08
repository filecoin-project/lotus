package kit

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

type DealHarness struct {
	t      *testing.T
	client *TestFullNode
	main   *TestMiner
	market *TestMiner
}

type MakeFullDealParams struct {
	Rseed                    int
	FastRet                  bool
	StartEpoch               abi.ChainEpoch
	UseCARFileForStorageDeal bool

	// SuspendUntilCryptoeconStable suspends deal-making, until cryptoecon
	// parameters are stabilised. This affects projected collateral, and tests
	// will fail in network version 13 and higher if deals are started too soon
	// after network birth.
	//
	// The reason is that the formula for collateral calculation takes
	// circulating supply into account:
	//
	//   [portion of power this deal will be] * [~1% of tokens].
	//
	// In the first epochs after genesis, the total circulating supply is
	// changing dramatically in percentual terms. Therefore, if the deal is
	// proposed too soon, by the time it gets published on chain, the quoted
	// provider collateral will no longer be valid.
	//
	// The observation is that deals fail with:
	//
	//   GasEstimateMessageGas error: estimating gas used: message execution
	//   failed: exit 16, reason: Provider collateral out of bounds. (RetCode=16)
	//
	// Enabling this will suspend deal-making until the network has reached a
	// height of 300.
	SuspendUntilCryptoeconStable bool
}

// NewDealHarness creates a test harness that contains testing utilities for deals.
func NewDealHarness(t *testing.T, client *TestFullNode, main *TestMiner, market *TestMiner) *DealHarness {
	return &DealHarness{
		t:      t,
		client: client,
		main:   main,
		market: market,
	}
}

// MakeOnlineDeal makes an online deal, generating a random file with the
// supplied seed, and setting the specified fast retrieval flag and start epoch
// on the storage deal. It returns when the deal is sealed.
//
// TODO: convert input parameters to struct, and add size as an input param.
func (dh *DealHarness) MakeOnlineDeal(ctx context.Context, params MakeFullDealParams) (deal *cid.Cid, res *api.ImportRes, path string) {
	if params.UseCARFileForStorageDeal {
		res, _, path = dh.client.ClientImportCARFile(ctx, params.Rseed, 200)
	} else {
		res, path = dh.client.CreateImportFile(ctx, params.Rseed, 0)
	}

	dh.t.Logf("FILE CID: %s", res.Root)

	if params.SuspendUntilCryptoeconStable {
		dh.t.Logf("deal-making suspending until cryptecon parameters have stabilised")
		ts := dh.client.WaitTillChain(ctx, HeightAtLeast(300))
		dh.t.Logf("deal-making continuing; current height is %d", ts.Height())
	}

	dp := dh.DefaultStartDealParams()
	dp.Data.Root = res.Root
	dp.DealStartEpoch = params.StartEpoch
	dp.FastRetrieval = params.FastRet
	deal = dh.StartDeal(ctx, dp)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)
	fmt.Printf("WAIT DEAL SEALEDS START\n")
	dh.WaitDealSealed(ctx, deal, false, false, nil)
	fmt.Printf("WAIT DEAL SEALEDS END\n")
	return deal, res, path
}

func (dh *DealHarness) DefaultStartDealParams() api.StartDealParams {
	dp := api.StartDealParams{
		Data:              &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync},
		EpochPrice:        types.NewInt(1000000),
		MinBlocksDuration: uint64(build.MinDealDuration),
	}

	var err error
	dp.Miner, err = dh.main.ActorAddress(context.Background())
	require.NoError(dh.t, err)

	dp.Wallet, err = dh.client.WalletDefaultAddress(context.Background())
	require.NoError(dh.t, err)

	return dp
}

// StartDeal starts a storage deal between the client and the miner.
func (dh *DealHarness) StartDeal(ctx context.Context, dealParams api.StartDealParams) *cid.Cid {
	dealProposalCid, err := dh.client.ClientStartDeal(ctx, &dealParams)
	require.NoError(dh.t, err)
	return dealProposalCid
}

// WaitDealSealed waits until the deal is sealed.
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

		mds, err := dh.market.MarketListIncompleteDeals(ctx)
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
	fmt.Printf("WAIT DEAL SEALED LOOP BROKEN\n")
}

// WaitDealSealedQuiet waits until the deal is sealed, without logging anything.
func (dh *DealHarness) WaitDealSealedQuiet(ctx context.Context, deal *cid.Cid, noseal, noSealStart bool, cb func()) {
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
			break loop
		}

		_, err = dh.market.MarketListIncompleteDeals(ctx)
		require.NoError(dh.t, err)

		time.Sleep(time.Second / 2)
		if cb != nil {
			cb()
		}
	}
}

func (dh *DealHarness) ExpectDealFailure(ctx context.Context, deal *cid.Cid, errs string) error {
	for {
		di, err := dh.client.ClientGetDealInfo(ctx, *deal)
		require.NoError(dh.t, err)

		switch di.State {
		case storagemarket.StorageDealAwaitingPreCommit, storagemarket.StorageDealSealing:
			return fmt.Errorf("deal is sealing, and we expected an error: %s", errs)
		case storagemarket.StorageDealProposalRejected:
			if strings.Contains(di.Message, errs) {
				return nil
			}
			return fmt.Errorf("unexpected error: %s ; expected: %s", di.Message, errs)
		case storagemarket.StorageDealFailing:
			if strings.Contains(di.Message, errs) {
				return nil
			}
			return fmt.Errorf("unexpected error: %s ; expected: %s", di.Message, errs)
		case storagemarket.StorageDealError:
			if strings.Contains(di.Message, errs) {
				return nil
			}
			return fmt.Errorf("unexpected error: %s ; expected: %s", di.Message, errs)
		case storagemarket.StorageDealActive:
			return errors.New("expected to get an error, but didn't get one")
		}

		mds, err := dh.market.MarketListIncompleteDeals(ctx)
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
	}
}

// WaitDealPublished waits until the deal is published.
func (dh *DealHarness) WaitDealPublished(ctx context.Context, deal *cid.Cid) {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	updates, err := dh.market.MarketGetDealUpdates(subCtx)
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
	snums, err := dh.main.SectorsList(ctx)
	require.NoError(dh.t, err)
	for _, snum := range snums {
		si, err := dh.main.SectorsStatus(ctx, snum, false)
		require.NoError(dh.t, err)

		dh.t.Logf("Sector state <%d>-[%d]:, %s", snum, si.SealProof, si.State)
		if si.State == api.SectorState(sealing.WaitDeals) {
			require.NoError(dh.t, dh.main.SectorStartSealing(ctx, snum))
		}

		dh.main.FlushSealingBatches(ctx)
	}
}

func (dh *DealHarness) PerformRetrieval(ctx context.Context, deal *cid.Cid, root cid.Cid, carExport bool) (path string) {
	// perform retrieval.
	info, err := dh.client.ClientGetDealInfo(ctx, *deal)
	require.NoError(dh.t, err)

	offers, err := dh.client.ClientFindData(ctx, root, &info.PieceCID)
	require.NoError(dh.t, err)
	require.NotEmpty(dh.t, offers, "no offers")

	carFile, err := ioutil.TempFile(dh.t.TempDir(), "ret-car")
	require.NoError(dh.t, err)

	defer carFile.Close() //nolint:errcheck

	caddr, err := dh.client.WalletDefaultAddress(ctx)
	require.NoError(dh.t, err)

	updatesCtx, cancel := context.WithCancel(ctx)
	updates, err := dh.client.ClientGetRetrievalUpdates(updatesCtx)
	require.NoError(dh.t, err)

	retrievalRes, err := dh.client.ClientRetrieve(ctx, offers[0].Order(caddr))
	require.NoError(dh.t, err)
consumeEvents:
	for {
		var evt api.RetrievalInfo
		select {
		case <-updatesCtx.Done():
			dh.t.Fatal("Retrieval Timed Out")
		case evt = <-updates:
			if evt.ID != retrievalRes.DealID {
				continue
			}
		}
		switch evt.Status {
		case retrievalmarket.DealStatusCompleted:
			break consumeEvents
		case retrievalmarket.DealStatusRejected:
			dh.t.Fatalf("Retrieval Proposal Rejected: %s", evt.Message)
		case
			retrievalmarket.DealStatusDealNotFound,
			retrievalmarket.DealStatusErrored:
			dh.t.Fatalf("Retrieval Error: %s", evt.Message)
		}
	}
	cancel()

	require.NoError(dh.t, dh.client.ClientExport(ctx,
		api.ExportRef{
			Root:   root,
			DealID: retrievalRes.DealID,
		},
		api.FileRef{
			Path:  carFile.Name(),
			IsCAR: carExport,
		}))

	ret := carFile.Name()
	if carExport {
		actualFile := dh.ExtractFileFromCAR(ctx, carFile)
		ret = actualFile.Name()
		_ = actualFile.Close() //nolint:errcheck
	}

	return ret
}

func (dh *DealHarness) ExtractFileFromCAR(ctx context.Context, file *os.File) (out *os.File) {
	bserv := dstest.Bserv()
	ch, err := car.LoadCar(bserv.Blockstore(), file)
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

	defer tmpfile.Close() //nolint:errcheck

	err = files.WriteTo(fil, tmpfile.Name())
	require.NoError(dh.t, err)

	return tmpfile
}

type RunConcurrentDealsOpts struct {
	N                        int
	FastRetrieval            bool
	CarExport                bool
	StartEpoch               abi.ChainEpoch
	UseCARFileForStorageDeal bool
}

func (dh *DealHarness) RunConcurrentDeals(opts RunConcurrentDealsOpts) {
	errgrp, _ := errgroup.WithContext(context.Background())
	for i := 0; i < opts.N; i++ {
		i := i
		errgrp.Go(func() (err error) {
			defer dh.t.Logf("finished concurrent deal %d/%d", i, opts.N)
			defer func() {
				// This is necessary because golang can't deal with test
				// failures being reported from children goroutines ¯\_(ツ)_/¯
				if r := recover(); r != nil {
					err = fmt.Errorf("deal failed: %s", r)
				}
			}()

			dh.t.Logf("making storage deal %d/%d", i, opts.N)

			deal, res, inPath := dh.MakeOnlineDeal(context.Background(), MakeFullDealParams{
				Rseed:                    5 + i,
				FastRet:                  opts.FastRetrieval,
				StartEpoch:               opts.StartEpoch,
				UseCARFileForStorageDeal: opts.UseCARFileForStorageDeal,
			})

			dh.t.Logf("retrieving deal %d/%d", i, opts.N)

			outPath := dh.PerformRetrieval(context.Background(), deal, res.Root, opts.CarExport)
			AssertFilesEqual(dh.t, inPath, outPath)
			return nil
		})
	}
	require.NoError(dh.t, errgrp.Wait())
}
