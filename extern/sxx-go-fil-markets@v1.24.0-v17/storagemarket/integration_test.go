package storagemarket_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipld/go-car"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-data-transfer/channelmonitor"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtnet "github.com/filecoin-project/go-data-transfer/network"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

var noOpDelay = testnodes.DelayFakeCommonNode{}

func TestMakeDeal(t *testing.T) {
	fixtureFiles := []string{"payload.txt", "duplicate_blocks.txt"}

	ctx := context.Background()
	testCases := map[string]struct {
		useStore        bool
		disableNewDeals bool
	}{
		"with stores": {
			useStore: true,
		},
		"with just blockstore": {
			useStore: false,
		},
		"disable new protocols": {
			useStore:        true,
			disableNewDeals: true,
		},
	}

	for _, fileName := range fixtureFiles {
		for testCase, data := range testCases {
			t.Run(testCase+"-"+filepath.Base(fileName), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				h := testharness.NewHarness(t, ctx, data.useStore, noOpDelay, noOpDelay, data.disableNewDeals, fileName)
				shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
				shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

				// set up a subscriber
				providerDealChan := make(chan storagemarket.MinerDeal)
				var checkedUnmarshalling bool
				subscriber := func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
					if !checkedUnmarshalling {
						// test that deal created can marshall and unmarshalled
						jsonBytes, err := json.Marshal(deal)
						require.NoError(t, err)
						var unmDeal storagemarket.MinerDeal
						err = json.Unmarshal(jsonBytes, &unmDeal)
						require.NoError(t, err)
						checkedUnmarshalling = true
					}
					providerDealChan <- deal
				}
				_ = h.Provider.SubscribeToEvents(subscriber)

				clientDealChan := make(chan storagemarket.ClientDeal)
				clientSubscriber := func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
					clientDealChan <- deal
				}
				_ = h.Client.SubscribeToEvents(clientSubscriber)

				// set ask price where we'll accept any price
				err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
				assert.NoError(t, err)

				result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, true, false)
				proposalCid := result.ProposalCid

				var providerSeenDeal storagemarket.MinerDeal
				var clientSeenDeal storagemarket.ClientDeal
				var providerstates, clientstates []storagemarket.StorageDealStatus
				for providerSeenDeal.State != storagemarket.StorageDealExpired ||
					clientSeenDeal.State != storagemarket.StorageDealExpired {
					select {
					case <-ctx.Done():
						t.Fatalf(`did not see all states before context closed
			saw client: %v,
			saw provider: %v`, dealStatesToStrings(clientstates), dealStatesToStrings(providerstates))
					case clientSeenDeal = <-clientDealChan:
						if len(clientstates) == 0 || clientSeenDeal.State != clientstates[len(clientstates)-1] {
							clientstates = append(clientstates, clientSeenDeal.State)
						}
					case providerSeenDeal = <-providerDealChan:
						if len(providerstates) == 0 || providerSeenDeal.State != providerstates[len(providerstates)-1] {
							providerstates = append(providerstates, providerSeenDeal.State)
						}
					}
				}

				expProviderStates := []storagemarket.StorageDealStatus{
					storagemarket.StorageDealValidating,
					storagemarket.StorageDealAcceptWait,
					storagemarket.StorageDealWaitingForData,
					storagemarket.StorageDealTransferring,
					storagemarket.StorageDealVerifyData,
					storagemarket.StorageDealReserveProviderFunds,
					storagemarket.StorageDealPublish,
					storagemarket.StorageDealPublishing,
					storagemarket.StorageDealStaged,
					storagemarket.StorageDealAwaitingPreCommit,
					storagemarket.StorageDealSealing,
					storagemarket.StorageDealFinalizing,
					storagemarket.StorageDealActive,
					storagemarket.StorageDealExpired,
				}

				expClientStates := []storagemarket.StorageDealStatus{
					storagemarket.StorageDealReserveClientFunds,
					//storagemarket.StorageDealClientFunding,  // skipped because funds available
					storagemarket.StorageDealFundsReserved,
					storagemarket.StorageDealStartDataTransfer,
					storagemarket.StorageDealTransferQueued,
					storagemarket.StorageDealTransferring,
					storagemarket.StorageDealCheckForAcceptance,
					storagemarket.StorageDealProposalAccepted,
					storagemarket.StorageDealAwaitingPreCommit,
					storagemarket.StorageDealSealing,
					storagemarket.StorageDealActive,
					storagemarket.StorageDealExpired,
				}

				assert.Equal(t, dealStatesToStrings(expProviderStates), dealStatesToStrings(providerstates))
				assert.Equal(t, dealStatesToStrings(expClientStates), dealStatesToStrings(clientstates))

				// check a couple of things to make sure we're getting the whole deal
				assert.Equal(t, h.TestData.Host1.ID(), providerSeenDeal.Client)
				assert.Empty(t, providerSeenDeal.Message)
				assert.Equal(t, proposalCid, providerSeenDeal.ProposalCid)
				assert.Equal(t, h.ProviderAddr, providerSeenDeal.ClientDealProposal.Proposal.Provider)

				cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
				assert.NoError(t, err)
				shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)
				assert.True(t, cd.FastRetrieval)

				providerDeals, err := h.Provider.ListLocalDeals()
				assert.NoError(t, err)

				pd := providerDeals[0]
				assert.Equal(t, proposalCid, pd.ProposalCid)
				assert.True(t, pd.FastRetrieval)
				shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)

				dl, err := h.Provider.GetLocalDeal(pd.ProposalCid)
				require.NoError(t, err)
				assert.True(t, dl.FastRetrieval)

				// test out query protocol
				status, err := h.Client.GetProviderDealState(ctx, proposalCid)
				assert.NoError(t, err)
				shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, status.State)
				assert.True(t, status.FastRetrieval)

				// ensure that the handoff has fast retrieval info
				assert.Len(t, h.ProviderNode.OnDealCompleteCalls, 1)
				assert.True(t, h.ProviderNode.OnDealCompleteCalls[0].FastRetrieval)
				h.ClientNode.VerifyExpectations(t)

				// ensure reference provider was called
				notifs := h.ReferenceProvider.GetNotifs()
				require.Len(t, notifs, 1)
				_, ok := notifs[string(proposalCid.Bytes())]
				require.True(t, ok)
			})
		}
	}
}

func TestMakeDealOffline(t *testing.T) {
	fixtureFiles := []string{
		filepath.Join(shared_testutil.ThisDir(t), "./fixtures/payload.txt"),
		filepath.Join(shared_testutil.ThisDir(t), "./fixtures/duplicate_blocks.txt"),
	}

	for _, file := range fixtureFiles {
		t.Run(filepath.Base(file), func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			h := testharness.NewHarness(t, ctx, true, noOpDelay, noOpDelay, false)

			shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

			commP, size, err := clientutils.CommP(ctx, h.Data, &storagemarket.DataRef{
				// hacky but need it for now because if it's manual, we wont get a CommP.
				TransferType: storagemarket.TTGraphsync,
				Root:         h.PayloadCid,
			}, 2<<29)
			require.NoError(t, err)

			dataRef := &storagemarket.DataRef{
				TransferType: storagemarket.TTManual,
				Root:         h.PayloadCid,
				PieceCid:     &commP,
				PieceSize:    size,
			}

			result := h.ProposeStorageDeal(t, dataRef, false, false)
			proposalCid := result.ProposalCid

			wg := sync.WaitGroup{}

			h.WaitForClientEvent(&wg, storagemarket.ClientEventDataTransferComplete)
			h.WaitForProviderEvent(&wg, storagemarket.ProviderEventDataRequested)
			waitGroupWait(ctx, &wg)

			cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
			assert.NoError(t, err)
			require.Eventually(t, func() bool {
				cd, _ = h.Client.GetLocalDeal(ctx, proposalCid)
				return cd.State == storagemarket.StorageDealCheckForAcceptance
			}, 1*time.Second, 100*time.Millisecond, "actual deal status is %s", storagemarket.DealStates[cd.State])

			providerDeals, err := h.Provider.ListLocalDeals()
			assert.NoError(t, err)

			pd := providerDeals[0]
			assert.True(t, pd.ProposalCid.Equals(proposalCid))
			shared_testutil.AssertDealState(t, storagemarket.StorageDealWaitingForData, pd.State)

			// Do a selective CARv1 traversal on the CARv2 file to get a
			// deterministic CARv1 that we can import on the miner side.
			sc := car.NewSelectiveCar(ctx, h.Data, []car.Dag{{Root: h.PayloadCid, Selector: selectorparse.CommonSelector_ExploreAllRecursively}})
			prepared, err := sc.Prepare()
			require.NoError(t, err)
			carBuf := new(bytes.Buffer)
			require.NoError(t, prepared.Write(carBuf))

			err = h.Provider.ImportDataForDeal(ctx, pd.ProposalCid, carBuf)
			require.NoError(t, err)

			h.WaitForClientEvent(&wg, storagemarket.ClientEventDealExpired)
			h.WaitForProviderEvent(&wg, storagemarket.ProviderEventDealExpired)
			waitGroupWait(ctx, &wg)

			require.Eventually(t, func() bool {
				cd, err = h.Client.GetLocalDeal(ctx, proposalCid)
				if err != nil {
					return false
				}
				if cd.State != storagemarket.StorageDealExpired {
					return false
				}

				providerDeals, err = h.Provider.ListLocalDeals()
				if err != nil {
					return false
				}

				pd = providerDeals[0]
				if !pd.ProposalCid.Equals(proposalCid) {
					return false
				}

				if pd.State != storagemarket.StorageDealExpired {
					return false
				}
				return true
			}, 5*time.Second, 500*time.Millisecond)

			cd, err = h.Client.GetLocalDeal(ctx, proposalCid)
			assert.NoError(t, err)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)

			providerDeals, err = h.Provider.ListLocalDeals()
			assert.NoError(t, err)

			pd = providerDeals[0]
			assert.True(t, pd.ProposalCid.Equals(proposalCid))
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
		})
	}
}

func TestMakeDealNonBlocking(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	h := testharness.NewHarness(t, ctx, true, noOpDelay, noOpDelay, false)

	testCids := shared_testutil.GenerateCids(2)

	h.ProviderNode.WaitForMessageBlocks = true
	h.ProviderNode.AddFundsCid = testCids[1]
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)

	h.ClientNode.AddFundsCid = testCids[0]
	shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)

	wg := sync.WaitGroup{}
	h.WaitForClientEvent(&wg, storagemarket.ClientEventDataTransferComplete)
	h.WaitForProviderEvent(&wg, storagemarket.ProviderEventFundingInitiated)
	waitGroupWait(ctx, &wg)

	cd, err := h.Client.GetLocalDeal(ctx, result.ProposalCid)
	assert.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealCheckForAcceptance, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	assert.NoError(t, err)

	// Provider should be blocking on waiting for funds to appear on chain
	pd := providerDeals[0]
	assert.Equal(t, result.ProposalCid, pd.ProposalCid)
	require.Eventually(t, func() bool {
		providerDeals, err := h.Provider.ListLocalDeals()
		assert.NoError(t, err)
		pd = providerDeals[0]
		return pd.State == storagemarket.StorageDealProviderFunding
	}, 1*time.Second, 100*time.Millisecond, "actual deal status is %s", storagemarket.DealStates[pd.State])
}

// TestRestartOnlyProviderDataTransfer tests that when the provider is shut
// down, the connection is broken and then the provider is restarted, the
// data transfer will resume and the deal will complete successfully.
func TestRestartOnlyProviderDataTransfer(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Configure data-transfer to retry connection
	dtClientNetRetry := dtnet.RetryParameters(time.Second, time.Second, 5, 1)
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	td.DTNet1 = dtnet.NewFromLibp2pHost(td.Host1, dtClientNetRetry)

	// Configure data-transfer to restart after stalling
	restartConf := dtimpl.ChannelRestartConfig(channelmonitor.Config{
		AcceptTimeout:          100 * time.Millisecond,
		RestartBackoff:         100 * time.Millisecond,
		RestartDebounce:        100 * time.Millisecond,
		MaxConsecutiveRestarts: 5,
		CompleteTimeout:        100 * time.Millisecond,
	})
	smState := testnodes.NewStorageMarketState()
	depGen := dependencies.NewDepGenerator()
	depGen.ClientNewDataTransfer = func(ds datastore.Batching, dir string, transferNetwork dtnet.DataTransferNetwork, transport datatransfer.Transport) (datatransfer.Manager, error) {
		return dtimpl.NewDataTransfer(ds, transferNetwork, transport, restartConf)
	}
	deps := depGen.New(t, ctx, td, smState, "", noOpDelay, noOpDelay)
	h := testharness.NewHarnessWithTestData(t, td, deps, true, false)

	client := h.Client
	host1 := h.TestData.Host1
	host2 := h.TestData.Host2

	// start client and provider
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
	shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

	// set ask price where we'll accept any price
	err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
	require.NoError(t, err)

	//h.DTClient.SubscribeToEvents(func(event datatransfer.Event, channelState datatransfer.ChannelState) {
	//	fmt.Printf("dt-clnt %s: %s %s\n", datatransfer.Events[event.Code], datatransfer.Statuses[channelState.Status()], channelState.Message())
	//})
	//h.DTProvider.SubscribeToEvents(func(event datatransfer.Event, channelState datatransfer.ChannelState) {
	//	fmt.Printf("dt-prov %s: %s %s\n", datatransfer.Events[event.Code], datatransfer.Statuses[channelState.Status()], channelState.Message())
	//})
	//
	//_ = client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
	//	fmt.Printf("%s: %s %s\n", storagemarket.ClientEvents[event], storagemarket.DealStates[deal.State], deal.Message)
	//})
	//_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
	//	fmt.Printf("Provider %s: %s\n", storagemarket.ProviderEvents[event], storagemarket.DealStates[deal.State])
	//})

	// wait for provider to enter deal transferring state and stop
	wg := sync.WaitGroup{}
	wg.Add(1)
	var providerState []storagemarket.MinerDeal
	h.DTClient.SubscribeToEvents(func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		if event.Code == datatransfer.Accept {
			t.Log("client has accepted data-transfer query, shutting down provider")

			require.NoError(t, h.TestData.MockNet.UnlinkPeers(host1.ID(), host2.ID()))
			require.NoError(t, h.TestData.MockNet.DisconnectPeers(host1.ID(), host2.ID()))
			require.NoError(t, h.Provider.Stop())

			// deal could have expired already on the provider side for the `ClientEventDealAccepted` event
			// so, we should wait on the `ProviderEventDealExpired` event ONLY if the deal has not expired.
			providerState, err = h.Provider.ListLocalDeals()
			assert.NoError(t, err)
			wg.Done()
		}
	})

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
	proposalCid := result.ProposalCid
	t.Log("storage deal proposed")

	waitGroupWait(ctx, &wg)
	t.Log("provider has been shutdown the first time")

	// Assert client state
	cd, err := client.GetLocalDeal(ctx, proposalCid)
	require.NoError(t, err)
	t.Logf("client state after stopping is %s", storagemarket.DealStates[cd.State])
	require.True(t, cd.State == storagemarket.StorageDealStartDataTransfer || cd.State == storagemarket.StorageDealTransferring)

	// Create new provider (but don't restart yet)
	newProvider := h.CreateNewProvider(t, ctx, h.TestData)

	t.Logf("provider state after stopping is %s", storagemarket.DealStates[providerState[0].State])
	require.Equal(t, storagemarket.StorageDealTransferring, providerState[0].State)

	// This wait group will complete after the deal has completed on both the
	// client and provider
	expireWg := sync.WaitGroup{}
	expireWg.Add(1)
	_ = newProvider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		//fmt.Printf("New Provider %s: %s\n", storagemarket.ProviderEvents[event], storagemarket.DealStates[deal.State])
		if event == storagemarket.ProviderEventDealExpired {
			expireWg.Done()
		}
	})

	expireWg.Add(1)
	_ = client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == storagemarket.ClientEventDealExpired {
			expireWg.Done()
		}
	})

	// sleep for a moment
	time.Sleep(1 * time.Second)
	t.Log("finished sleeping")

	// Restore connection, go-data-transfer should try to reconnect
	require.NoError(t, h.TestData.MockNet.LinkAll())
	time.Sleep(200 * time.Millisecond)
	conn, err := h.TestData.MockNet.ConnectPeers(host1.ID(), host2.ID())
	require.NoError(t, err)
	require.NotNil(t, conn)

	// Restart the provider
	shared_testutil.StartAndWaitForReady(ctx, t, newProvider)
	t.Log("------- provider has been restarted---------")

	// -------------------------------------------------------------------
	// How to restart manually - shouldn't be needed as the data-transfer
	// module will restart automatically, but leaving it here in case it's
	// needed for debugging in future.
	//chs, err := h.DTClient.InProgressChannels(ctx)
	//require.Len(t, chs, 1)
	//for chid := range chs {
	//	h.DTClient.RestartDataTransferChannel(ctx, chid)
	//}
	// -------------------------------------------------------------------

	// Wait till both client and provider have completed the deal
	waitGroupWait(ctx, &expireWg)
	t.Log("---------- finished waiting for expected events-------")

	// Ensure the client and provider both reached the final state
	cd, err = client.GetLocalDeal(ctx, proposalCid)
	require.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)

	providerDeals, err := newProvider.ListLocalDeals()
	require.NoError(t, err)

	pd := providerDeals[0]
	require.Equal(t, pd.ProposalCid, proposalCid)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
}

// TestRestartProviderAtPublishStage tests that if the provider is restarted
// when it's in the publish state, it will successfully complete the deal
func TestRestartProviderAtPublishStage(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	td := shared_testutil.NewLibp2pTestData(ctx, t)
	smState := testnodes.NewStorageMarketState()
	depGen := dependencies.NewDepGenerator()
	deps := depGen.New(t, ctx, td, smState, "", noOpDelay, noOpDelay)
	h := testharness.NewHarnessWithTestData(t, td, deps, true, false)

	// start client and provider
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
	shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

	// set ask price where we'll accept any price
	err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
	require.NoError(t, err)

	// Listen for when the provider reaches the Publish state, and shut it down
	wgProviderPublish := sync.WaitGroup{}
	wgProviderPublish.Add(1)
	var providerState []storagemarket.MinerDeal
	h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		t.Logf("Provider %s: %s\n", storagemarket.ProviderEvents[event], storagemarket.DealStates[deal.State])
		if deal.State == storagemarket.StorageDealPublish {
			require.NoError(t, h.Provider.Stop())

			time.Sleep(time.Second)

			providerState, err = h.Provider.ListLocalDeals()
			assert.NoError(t, err)
			wgProviderPublish.Done()
		}
	})

	// Propose a storage deal
	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
	proposalCid := result.ProposalCid
	t.Log("storage deal proposed")

	// Wait till the deal reaches the Publish state
	waitGroupWait(ctx, &wgProviderPublish)
	t.Log("provider has been shutdown")

	// Create new provider (but don't restart yet)
	newProvider := h.CreateNewProvider(t, ctx, h.TestData)

	t.Logf("provider state after stopping is %s", storagemarket.DealStates[providerState[0].State])

	// This wait group will complete after the deal has completed on both the
	// client and provider
	expireWg := sync.WaitGroup{}
	expireWg.Add(1)
	_ = newProvider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		t.Logf("New Provider %s: %s\n", storagemarket.ProviderEvents[event], storagemarket.DealStates[deal.State])
		if event == storagemarket.ProviderEventDealExpired {
			expireWg.Done()
		}
	})

	expireWg.Add(1)
	_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == storagemarket.ClientEventDealExpired {
			expireWg.Done()
		}
	})

	// sleep for a moment
	time.Sleep(1 * time.Second)
	t.Log("finished sleeping")

	// Restart the provider
	err = newProvider.Start(ctx)
	require.NoError(t, err)
	t.Log("------- provider has been restarted---------")

	// Wait till both client and provider have completed the deal
	waitGroupWait(ctx, &expireWg)
	t.Log("---------- finished waiting for expected events-------")

	// Ensure the provider reached the final state
	providerDeals, err := newProvider.ListLocalDeals()
	require.NoError(t, err)

	pd := providerDeals[0]
	require.Equal(t, pd.ProposalCid, proposalCid)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
}

// FIXME Gets hung sometimes
func TestRestartClient(t *testing.T) {
	testCases := map[string]struct {
		stopAtClientEvent   storagemarket.ClientEvent
		stopAtProviderEvent storagemarket.ProviderEvent

		expectedClientState storagemarket.StorageDealStatus
		clientDelay         testnodes.DelayFakeCommonNode
		providerDelay       testnodes.DelayFakeCommonNode
	}{

		"ClientEventDataTransferInitiated": {
			// This test can fail if client crashes without seeing a Provider DT complete
			// See https://github.com/filecoin-project/lotus/issues/3966
			stopAtClientEvent:   storagemarket.ClientEventDataTransferInitiated,
			expectedClientState: storagemarket.StorageDealTransferring,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		"ClientEventDataTransferComplete": {
			stopAtClientEvent:   storagemarket.ClientEventDataTransferComplete,
			stopAtProviderEvent: storagemarket.ProviderEventDataTransferCompleted,
			expectedClientState: storagemarket.StorageDealCheckForAcceptance,
		},

		"ClientEventFundingComplete": {
			// Edge case: Provider begins the state machine on receiving a deal stream request
			// client crashes -> restarts -> sends deal stream again -> state machine fails
			// See https://github.com/filecoin-project/lotus/issues/3966
			stopAtClientEvent:   storagemarket.ClientEventFundingComplete,
			expectedClientState: storagemarket.StorageDealFundsReserved,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		// FIXME
		"ClientEventInitiateDataTransfer": { // works well but sometimes state progresses beyond StorageDealStartDataTransfer
			stopAtClientEvent:   storagemarket.ClientEventInitiateDataTransfer,
			expectedClientState: storagemarket.StorageDealStartDataTransfer,
			clientDelay:         noOpDelay,
			providerDelay:       noOpDelay,
		},

		"ClientEventDealAccepted": { // works well
			stopAtClientEvent:   storagemarket.ClientEventDealAccepted,
			expectedClientState: storagemarket.StorageDealProposalAccepted,
			clientDelay:         testnodes.DelayFakeCommonNode{ValidatePublishedDeal: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},

		"ClientEventDealActivated": { // works well
			stopAtClientEvent:   storagemarket.ClientEventDealActivated,
			expectedClientState: storagemarket.StorageDealActive,
			clientDelay:         testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},

		"ClientEventDealPublished": { // works well
			stopAtClientEvent:   storagemarket.ClientEventDealPublished,
			expectedClientState: storagemarket.StorageDealSealing,
			clientDelay:         testnodes.DelayFakeCommonNode{OnDealSectorCommitted: true},
			providerDelay:       testnodes.DelayFakeCommonNode{OnDealExpiredOrSlashed: true},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
			defer cancel()
			h := testharness.NewHarness(t, ctx, true, tc.clientDelay, tc.providerDelay, false)
			host1 := h.TestData.Host1
			host2 := h.TestData.Host2

			shared_testutil.StartAndWaitForReady(ctx, t, h.Client)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)

			// set ask price where we'll accept any price
			err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
			require.NoError(t, err)

			wg := sync.WaitGroup{}
			wg.Add(1)
			var providerState []storagemarket.MinerDeal
			_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				if event == tc.stopAtClientEvent {
					// Stop the client and provider at some point during deal negotiation
					ev := storagemarket.ClientEvents[event]
					t.Logf("event %s has happened on client, shutting down client and provider", ev)
					require.NoError(t, h.Client.Stop())
					require.NoError(t, h.TestData.MockNet.UnlinkPeers(host1.ID(), host2.ID()))
					require.NoError(t, h.TestData.MockNet.DisconnectPeers(host1.ID(), host2.ID()))

					// if a provider stop event isn't specified, just stop the provider here
					if tc.stopAtProviderEvent == 0 {
						require.NoError(t, h.Provider.Stop())
					}

					// deal could have expired already on the provider side for the `ClientEventDealAccepted` event
					// so, we should wait on the `ProviderEventDealExpired` event ONLY if the deal has not expired.
					providerState, err = h.Provider.ListLocalDeals()
					assert.NoError(t, err)
					wg.Done()
				}
			})

			// if this test case specifies a provider stop event...
			if tc.stopAtProviderEvent != 0 {
				wg.Add(1)

				_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
					if event == tc.stopAtProviderEvent {
						require.NoError(t, h.Provider.Stop())
						wg.Done()
					}
				})
			}

			result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
			proposalCid := result.ProposalCid
			t.Log("storage deal proposed")

			waitGroupWait(ctx, &wg)
			t.Log("both client and provider have been shutdown the first time")

			cd, err := h.Client.GetLocalDeal(ctx, proposalCid)
			require.NoError(t, err)
			t.Logf("client state after stopping is %s", storagemarket.DealStates[cd.State])
			if tc.expectedClientState != cd.State {
				t.Logf("client state message: %s", cd.Message)
				require.Fail(t, fmt.Sprintf("client deal state mismatch:\nexpected: %s\nactual:   %s",
					storagemarket.DealStates[tc.expectedClientState], storagemarket.DealStates[cd.State]))
			}

			deps := dependencies.NewDependenciesWithTestData(t, ctx, h.TestData, h.SMState, "", noOpDelay, noOpDelay)
			h = testharness.NewHarnessWithTestData(t, h.TestData, deps, true, false)

			if len(providerState) == 0 {
				t.Log("no deal created on provider after stopping")
			} else {
				t.Logf("provider state after stopping is %s", storagemarket.DealStates[providerState[0].State])
			}

			if len(providerState) == 0 || providerState[0].State != storagemarket.StorageDealExpired {
				wg.Add(1)
				_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
					if deal.State == storagemarket.StorageDealError {
						t.Errorf("storage deal provider error: %s", deal.Message)
						wg.Done()
					}
					if event == storagemarket.ProviderEventDealExpired {
						wg.Done()
					}
				})
			}
			wg.Add(1)
			_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				if deal.State == storagemarket.StorageDealError {
					t.Errorf("storage deal client error: %s", deal.Message)
					wg.Done()
				}
				if event == storagemarket.ClientEventDealExpired {
					wg.Done()
				}
			})

			require.NoError(t, h.TestData.MockNet.LinkAll())
			time.Sleep(200 * time.Millisecond)
			conn, err := h.TestData.MockNet.ConnectPeers(host1.ID(), host2.ID())
			require.NoError(t, err)
			require.NotNil(t, conn)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
			shared_testutil.StartAndWaitForReady(ctx, t, h.Client)
			t.Log("------- client and provider have been restarted---------")
			waitGroupWait(ctx, &wg)
			t.Log("---------- finished waiting for expected events-------")

			cd, err = h.Client.GetLocalDeal(ctx, proposalCid)
			require.NoError(t, err)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)

			providerDeals, err := h.Provider.ListLocalDeals()
			require.NoError(t, err)

			pd := providerDeals[0]
			require.Equal(t, pd.ProposalCid, proposalCid)
			shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
		})
	}
}

// TestBounceConnectionDataTransfer tests that when the the connection is
// broken and then restarted, the data transfer will resume and the deal will
// complete successfully.
func TestBounceConnectionDataTransfer(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Configure data-transfer to make 5 attempts, backing off 1s each time
	dtClientNetRetry := dtnet.RetryParameters(time.Second, time.Second, 5, 1)
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	td.DTNet1 = dtnet.NewFromLibp2pHost(td.Host1, dtClientNetRetry)

	// Configure data-transfer to automatically restart when connection goes down
	restartConf := dtimpl.ChannelRestartConfig(channelmonitor.Config{
		AcceptTimeout:          100 * time.Millisecond,
		RestartBackoff:         100 * time.Millisecond,
		RestartDebounce:        100 * time.Millisecond,
		MaxConsecutiveRestarts: 5,
		CompleteTimeout:        100 * time.Millisecond,
	})
	smState := testnodes.NewStorageMarketState()
	depGen := dependencies.NewDepGenerator()
	depGen.ClientNewDataTransfer = func(ds datastore.Batching, dir string, transferNetwork dtnet.DataTransferNetwork, transport datatransfer.Transport) (datatransfer.Manager, error) {
		return dtimpl.NewDataTransfer(ds, transferNetwork, transport, restartConf)
	}
	deps := depGen.New(t, ctx, td, smState, "", noOpDelay, noOpDelay)
	h := testharness.NewHarnessWithTestData(t, td, deps, true, false)

	client := h.Client
	clientHost := h.TestData.Host1.ID()
	providerHost := h.TestData.Host2.ID()

	// start client and provider
	shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
	shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

	// set ask price where we'll accept any price
	err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
	require.NoError(t, err)

	// Bounce connection after this many bytes have been queued for sending
	bounceConnectionAt := map[uint64]bool{
		1000: false,
		5000: false,
	}
	h.DTClient.SubscribeToEvents(func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		//t.Logf("dt-clnt %s: %s %s\n", datatransfer.Events[event.Code], datatransfer.Statuses[channelState.Status()], channelState.Message())
		if event.Code == datatransfer.DataQueuedProgress {
			//t.Logf("  > qued %d", channelState.Queued())

			// Check if enough bytes have been queued that the connection
			// should be bounced
			for at, already := range bounceConnectionAt {
				if channelState.Sent() > at && !already {
					bounceConnectionAt[at] = true

					// Break the connection
					queued := channelState.Queued()
					t.Logf("    breaking connection after sending %d bytes", queued)
					h.TestData.MockNet.DisconnectPeers(clientHost, providerHost)
					h.TestData.MockNet.UnlinkPeers(clientHost, providerHost)

					go func() {
						// Restore the connection
						time.Sleep(100 * time.Millisecond)
						t.Logf("    restoring connection from bounce at %d bytes", queued)
						h.TestData.MockNet.LinkPeers(clientHost, providerHost)
					}()
				}
			}
		}
		//if event.Code == datatransfer.DataSentProgress {
		//	t.Logf("  > sent %d", channelState.Sent())
		//}
	})
	//h.DTProvider.SubscribeToEvents(func(event datatransfer.Event, channelState datatransfer.ChannelState) {
	//	if event.Code == datatransfer.DataReceivedProgress {
	//		t.Logf("  > rcvd %d", channelState.Received())
	//	}
	//})

	result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
	proposalCid := result.ProposalCid
	t.Log("storage deal proposed")

	// This wait group will complete after the deal has completed on both the
	// client and provider
	expireWg := sync.WaitGroup{}
	expireWg.Add(1)
	_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if event == storagemarket.ProviderEventDealExpired {
			expireWg.Done()
		}
	})

	expireWg.Add(1)
	_ = client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == storagemarket.ClientEventDealExpired {
			expireWg.Done()
		}
	})

	// Wait till both client and provider have completed the deal
	waitGroupWait(ctx, &expireWg)
	t.Log("---------- finished waiting for expected events-------")

	// Ensure the client and provider both reached the final state
	cd, err := client.GetLocalDeal(ctx, proposalCid)
	require.NoError(t, err)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, cd.State)

	providerDeals, err := h.Provider.ListLocalDeals()
	require.NoError(t, err)

	pd := providerDeals[0]
	require.Equal(t, pd.ProposalCid, proposalCid)
	shared_testutil.AssertDealState(t, storagemarket.StorageDealExpired, pd.State)
}

// TestCancelDataTransfer tests that cancelling a data transfer cancels the deal
func TestCancelDataTransfer(t *testing.T) {
	run := func(t *testing.T, cancelByClient bool, hasConnectivity bool) {
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		h := testharness.NewHarness(t, ctx, true, noOpDelay, noOpDelay, false)
		client := h.Client
		provider := h.Provider
		host1 := h.TestData.Host1
		host2 := h.TestData.Host2

		// start client and provider
		shared_testutil.StartAndWaitForReady(ctx, t, h.Provider)
		shared_testutil.StartAndWaitForReady(ctx, t, h.Client)

		// set ask price where we'll accept any price
		err := h.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
		require.NoError(t, err)

		// wait for client to start transferring data
		wg := sync.WaitGroup{}
		wg.Add(1)
		_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
			if event == storagemarket.ClientEventDataTransferInitiated {
				ev := storagemarket.ClientEvents[event]
				t.Logf("event %s has happened on client", ev)

				if !hasConnectivity {
					t.Logf("disconnecting client and provider")
					// Simulate the connection to the remote peer going down
					require.NoError(t, h.TestData.MockNet.UnlinkPeers(host1.ID(), host2.ID()))
					require.NoError(t, h.TestData.MockNet.DisconnectPeers(host1.ID(), host2.ID()))
				}

				wg.Done()
			}
		})

		result := h.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: h.PayloadCid}, false, false)
		proposalCid := result.ProposalCid
		t.Log("storage deal proposed")

		waitGroupWait(ctx, &wg)
		if !hasConnectivity {
			t.Log("network has been disconnected")
		}

		// Assert client is transferring data
		cd, err := client.GetLocalDeal(ctx, proposalCid)
		require.NoError(t, err)
		t.Logf("client state after stopping is %s", storagemarket.DealStates[cd.State])
		require.True(t, cd.State == storagemarket.StorageDealStartDataTransfer || cd.State == storagemarket.StorageDealTransferring)

		// Keep track of client states
		var clientErroredOut sync.WaitGroup
		var clientstates []storagemarket.StorageDealStatus

		// Client will only move to error state if
		// - client initiates cancel
		// - client receives cancel message from provider
		if cancelByClient || hasConnectivity {
			clientErroredOut.Add(1)
			_ = h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
				if len(clientstates) == 0 || deal.State != clientstates[len(clientstates)-1] {
					clientstates = append(clientstates, deal.State)
				}

				if deal.State == storagemarket.StorageDealError {
					clientErroredOut.Done()
				}
			})
		}

		// Keep track of provider states
		var providerErroredOut sync.WaitGroup
		var providerstates []storagemarket.StorageDealStatus

		// Provider will only move to error state if
		// - provider initiates cancel
		// - provider receives cancel message from client
		if !cancelByClient || hasConnectivity {
			providerErroredOut.Add(1)
			_ = h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
				if len(providerstates) == 0 || deal.State != providerstates[len(providerstates)-1] {
					providerstates = append(providerstates, deal.State)
				}

				if deal.State == storagemarket.StorageDealError {
					providerErroredOut.Done()
				}
			})
		}

		// Should be one in-progress channel
		chans, err := h.DTClient.InProgressChannels(ctx)
		require.NoError(t, err)
		require.Len(t, chans, 1)
		for _, ch := range chans {
			require.Equal(t, datatransfer.Ongoing, ch.Status())

			dt := h.DTClient
			if !cancelByClient {
				dt = h.DTProvider
			}

			// Simulate data transfer channel being cancelled
			chid := ch.ChannelID()
			if hasConnectivity {
				err := dt.CloseDataTransferChannel(ctx, chid)
				require.NoError(t, err)
			} else {
				// If the network is down, use a short timeout so that the test
				// doesn't take too long to complete
				ctx, closeCtxCancel := context.WithTimeout(ctx, 100*time.Millisecond)
				defer closeCtxCancel()
				_ = dt.CloseDataTransferChannel(ctx, chid)
			}
		}

		// Wait for the state machines to reach the error state
		waitGroupWait(ctx, &clientErroredOut)
		waitGroupWait(ctx, &providerErroredOut)

		// Make sure state machine passed through expected states
		possStates := []storagemarket.StorageDealStatus{
			storagemarket.StorageDealTransferring,
			storagemarket.StorageDealFailing,
			storagemarket.StorageDealError,
		}
		expClientStates := possStates[len(possStates)-len(clientstates):]
		assert.Equal(t, dealStatesToStrings(expClientStates), dealStatesToStrings(clientstates))
		expProviderStates := possStates[len(possStates)-len(providerstates):]
		assert.Equal(t, dealStatesToStrings(expProviderStates), dealStatesToStrings(providerstates))

		// Verify the error message for the deal is correct
		if cancelByClient || hasConnectivity {
			deals, err := client.ListLocalDeals(ctx)
			require.NoError(t, err)
			assert.Len(t, deals, 1)
			assert.Equal(t, "data transfer cancelled", deals[0].Message)
		}
		if !cancelByClient || hasConnectivity {
			pdeals, err := provider.ListLocalDeals()
			require.NoError(t, err)
			assert.Len(t, pdeals, 1)
			assert.Equal(t, "data transfer cancelled", pdeals[0].Message)
		}
	}

	t.Run("client cancel request good connectivity", func(t *testing.T) {
		run(t, true, true)
	})
	t.Run("client cancel request no connectivity", func(t *testing.T) {
		run(t, true, false)
	})
	t.Run("provider cancel request good connectivity", func(t *testing.T) {
		run(t, false, true)
	})
	t.Run("provider cancel request no connectivity", func(t *testing.T) {
		run(t, false, false)
	})
}

// waitGroupWait calls wg.Wait while respecting context cancellation
func waitGroupWait(ctx context.Context, wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

func dealStatesToStrings(states []storagemarket.StorageDealStatus) []string {
	var out []string
	for _, state := range states {
		out = append(out, storagemarket.DealStates[state])
	}
	return out
}
