package retrievalmarket_test

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	logger "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-data-transfer/channelmonitor"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtnet "github.com/filecoin-project/go-data-transfer/network"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	testnodes2 "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

var log = logger.Logger("restart_test")

var noOpDelay = testnodes.DelayFakeCommonNode{}

// TODO
// TEST CONNECTION BOUNCE FOR ALL MEANINGFUL STATES OF THE CLIENT AND PROVIDER DEAL LIFECYCLE.
// CURRENTLY, WE ONLY TEST THIS FOR THE DEALSTATUS ONGOING STATE.

// TestBounceConnectionDealTransferOngoing tests that when the the connection is
// broken and then restarted during deal data transfer for an ongoing deal, the data transfer will resume and the deal will
// complete successfully.
func TestBounceConnectionDealTransferOngoing(t *testing.T) {
	bgCtx := context.Background()
	logger.SetLogLevel("restart_test", "debug")
	//logger.SetLogLevel("dt-impl", "debug")
	//logger.SetLogLevel("dt-chanmon", "debug")
	//logger.SetLogLevel("dt_graphsync", "debug")
	//logger.SetLogLevel("markets-rtvl", "debug")
	//logger.SetLogLevel("markets-rtvl-reval", "debug")

	tcs := map[string]struct {
		unSealPrice             abi.TokenAmount
		pricePerByte            abi.TokenAmount
		paymentInterval         uint64
		paymentIntervalIncrease uint64
		voucherAmts             []abi.TokenAmount
		maxVoucherAmt           abi.TokenAmount
	}{
		"non-zero unseal, non zero price per byte": {
			unSealPrice:             abi.NewTokenAmount(1000),
			pricePerByte:            abi.NewTokenAmount(1000),
			paymentInterval:         uint64(10000),
			paymentIntervalIncrease: uint64(1000),
			maxVoucherAmt:           abi.NewTokenAmount(19959000),
		},

		"zero unseal, non-zero price per byte": {
			unSealPrice:             big.Zero(),
			pricePerByte:            abi.NewTokenAmount(1000),
			paymentInterval:         uint64(10000),
			paymentIntervalIncrease: uint64(1000),
			maxVoucherAmt:           abi.NewTokenAmount(19958000),
		},

		"zero unseal, zero price per byte": {
			unSealPrice:             big.Zero(),
			pricePerByte:            big.Zero(),
			paymentInterval:         uint64(0),
			paymentIntervalIncrease: uint64(0),
			maxVoucherAmt:           abi.NewTokenAmount(0),
		},

		"non-zero unseal, zero price per byte": {
			unSealPrice:   abi.NewTokenAmount(1000),
			pricePerByte:  big.Zero(),
			maxVoucherAmt: abi.NewTokenAmount(1000),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			dtClientNetRetry := dtnet.RetryParameters(time.Second, time.Second, 5, 1)
			restartConf := dtimpl.ChannelRestartConfig(channelmonitor.Config{
				AcceptTimeout:          100 * time.Millisecond,
				RestartBackoff:         100 * time.Millisecond,
				RestartDebounce:        100 * time.Millisecond,
				MaxConsecutiveRestarts: 5,
				CompleteTimeout:        100 * time.Millisecond,
			})
			td := shared_testutil.NewLibp2pTestData(bgCtx, t)
			td.DTNet1 = dtnet.NewFromLibp2pHost(td.Host1, dtClientNetRetry)
			depGen := dependencies.NewDepGenerator()
			depGen.ClientNewDataTransfer = func(ds datastore.Batching, dir string, transferNetwork dtnet.DataTransferNetwork, transport datatransfer.Transport) (datatransfer.Manager, error) {
				return dtimpl.NewDataTransfer(ds, transferNetwork, transport, restartConf)
			}
			deps := depGen.New(t, bgCtx, td, testnodes.NewStorageMarketState(), "", noOpDelay, noOpDelay)
			providerNode := testnodes2.NewTestRetrievalProviderNode()
			sa := testnodes2.NewTestSectorAccessor()
			pieceStore := shared_testutil.NewTestPieceStore()
			dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)
			deps.DagStore = dagStore

			sh := testharness.NewHarnessWithTestData(t, deps.TestData, deps, true, false)

			// do a storage deal
			storageClientSeenDeal := doStorage(t, bgCtx, sh)
			ctxTimeout, canc := context.WithTimeout(bgCtx, 5*time.Second)
			defer canc()

			// create a retrieval test harness
			params := retrievalmarket.Params{
				UnsealPrice:             tc.unSealPrice,
				PricePerByte:            tc.pricePerByte,
				PaymentInterval:         tc.paymentInterval,
				PaymentIntervalIncrease: tc.paymentIntervalIncrease,
			}
			rh := newRetrievalHarnessWithDeps(ctxTimeout, t, sh, storageClientSeenDeal, providerNode, sa, pieceStore, dagStore, params)
			clientHost := rh.TestDataNet.Host1.ID()
			providerHost := rh.TestDataNet.Host2.ID()

			// Bounce connection after this many bytes have been queued for sending
			bounceConnectionAt := map[uint64]bool{
				1000: false,
				3000: false,
				5000: false,
				7000: false,
				9000: false,
			}

			sh.DTProvider.SubscribeToEvents(func(event datatransfer.Event, channelState datatransfer.ChannelState) {
				if event.Code == datatransfer.DataQueuedProgress {
					log.Debugf("DataQueuedProgress %d", channelState.Queued())
					// Check if enough bytes have been queued that the connection
					// should be bounced
					for at, already := range bounceConnectionAt {
						if channelState.Queued() > at && !already {
							bounceConnectionAt[at] = true

							// Break the connection
							queued := channelState.Queued()
							sent := channelState.Sent()
							t.Logf("breaking connection at queue %d sent %d bytes", queued, sent)
							rh.TestDataNet.MockNet.DisconnectPeers(clientHost, providerHost)
							rh.TestDataNet.MockNet.UnlinkPeers(clientHost, providerHost)

							go func() {
								time.Sleep(100 * time.Millisecond)
								t.Logf("restoring connection at queue %d sent %d bytes", queued, sent)
								rh.TestDataNet.MockNet.LinkPeers(clientHost, providerHost)
							}()
						}
					}
				}
				if event.Code == datatransfer.DataSent {
					log.Debugf("DataSent %d", channelState.Sent())
				}
				if event.Code == datatransfer.DataSentProgress {
					log.Debugf("DataSentProgress %d", channelState.Sent())
				}
			})
			sh.DTClient.SubscribeToEvents(func(event datatransfer.Event, channelState datatransfer.ChannelState) {
				if event.Code == datatransfer.DataReceived {
					log.Debugf("DataReceived %d", channelState.Received())
				}
				if event.Code == datatransfer.DataReceivedProgress {
					log.Debugf("DataReceivedProgress %d", channelState.Received())
				}
			})

			checkRetrieve(t, bgCtx, rh, sh, tc.voucherAmts)
			require.Equal(t, tc.maxVoucherAmt, rh.ProviderNode.MaxReceivedVoucher())
		})
	}
}

// TestBounceConnectionDealTransferUnsealing tests that when the the connection
// is broken and then restarted during unsealing, the data transfer will resume
// and the deal will complete successfully.
func TestBounceConnectionDealTransferUnsealing(t *testing.T) {
	bgCtx := context.Background()
	//logger.SetLogLevel("dt-chanmon", "debug")
	//logger.SetLogLevel("retrieval", "debug")
	//logger.SetLogLevel("retrievalmarket_impl", "debug")
	logger.SetLogLevel("restart_test", "debug")
	//logger.SetLogLevel("markets-rtvl-reval", "debug")
	//logger.SetLogLevel("graphsync", "debug")
	//logger.SetLogLevel("gs-traversal", "debug")
	//logger.SetLogLevel("gs-executor", "debug")

	beforeRestoringConnection := true
	afterRestoringConnection := !beforeRestoringConnection
	tcs := []struct {
		name         string
		finishUnseal bool
	}{{
		name:         "finish unseal before restoring connection",
		finishUnseal: beforeRestoringConnection,
	}, {
		name:         "finish unseal after restoring connection",
		finishUnseal: afterRestoringConnection,
	}}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			restartComplete := make(chan struct{})
			onRestartComplete := func(_ datatransfer.ChannelID) {
				close(restartComplete)
			}

			dtClientNetRetry := dtnet.RetryParameters(time.Second, time.Second, 5, 1)
			restartConf := dtimpl.ChannelRestartConfig(channelmonitor.Config{
				AcceptTimeout:          100 * time.Millisecond,
				RestartBackoff:         100 * time.Millisecond,
				RestartDebounce:        100 * time.Millisecond,
				MaxConsecutiveRestarts: 5,
				CompleteTimeout:        100 * time.Millisecond,
				OnRestartComplete:      onRestartComplete,
			})
			td := shared_testutil.NewLibp2pTestData(bgCtx, t)
			td.DTNet1 = dtnet.NewFromLibp2pHost(td.Host1, dtClientNetRetry)
			depGen := dependencies.NewDepGenerator()
			depGen.ClientNewDataTransfer = func(ds datastore.Batching, dir string, transferNetwork dtnet.DataTransferNetwork, transport datatransfer.Transport) (datatransfer.Manager, error) {
				return dtimpl.NewDataTransfer(ds, transferNetwork, transport, restartConf)
			}
			deps := depGen.New(t, bgCtx, td, testnodes.NewStorageMarketState(), "", noOpDelay, noOpDelay)
			providerNode := testnodes2.NewTestRetrievalProviderNode()
			sa := testnodes2.NewTestSectorAccessor()
			pieceStore := shared_testutil.NewTestPieceStore()
			dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)
			deps.DagStore = dagStore

			sh := testharness.NewHarnessWithTestData(t, td, deps, true, false)

			// do a storage deal
			storageClientSeenDeal := doStorage(t, bgCtx, sh)
			ctxTimeout, canc := context.WithTimeout(bgCtx, 5*time.Second)
			defer canc()

			// create a retrieval test harness
			maxVoucherAmt := abi.NewTokenAmount(19959000)
			params := retrievalmarket.Params{
				UnsealPrice:             abi.NewTokenAmount(1000),
				PricePerByte:            abi.NewTokenAmount(1000),
				PaymentInterval:         uint64(10000),
				PaymentIntervalIncrease: uint64(1000),
			}
			rh := newRetrievalHarnessWithDeps(ctxTimeout, t, sh, storageClientSeenDeal, providerNode, sa, pieceStore, dagStore, params)
			clientHost := rh.TestDataNet.Host1.ID()
			providerHost := rh.TestDataNet.Host2.ID()

			// Pause unsealing
			rh.SectorAccessor.PauseUnseal()

			firstPayRcvd := false
			rh.Provider.SubscribeToEvents(func(event retrievalmarket.ProviderEvent, state retrievalmarket.ProviderDealState) {
				// When the provider receives the first payment from the
				// client (the payment for unsealing), the provider moves
				// to the unsealing state
				if event != retrievalmarket.ProviderEventPaymentReceived || firstPayRcvd {
					return
				}

				firstPayRcvd = true

				log.Debugf("breaking connection at %s", retrievalmarket.ProviderEvents[event])
				rh.TestDataNet.MockNet.DisconnectPeers(clientHost, providerHost)
				rh.TestDataNet.MockNet.UnlinkPeers(clientHost, providerHost)

				go func() {
					// Simulate unsealing delay
					time.Sleep(50 * time.Millisecond)

					// If unsealing should finish before restoring the connection
					if tc.finishUnseal == beforeRestoringConnection {
						// Finish unsealing
						log.Debugf("finish unseal")
						rh.SectorAccessor.FinishUnseal()
						time.Sleep(20 * time.Millisecond)
					}

					// Restore the connection
					log.Debugf("restoring connection")
					rh.TestDataNet.MockNet.LinkPeers(clientHost, providerHost)

					// If unsealing should finish after restoring the connection
					if tc.finishUnseal == afterRestoringConnection {
						// Wait for the Restart message to be sent and
						// acknowledged
						select {
						case <-ctxTimeout.Done():
							return
						case <-restartComplete:
						}

						// Finish unsealing
						time.Sleep(50 * time.Millisecond)
						log.Debugf("finish unseal")
						rh.SectorAccessor.FinishUnseal()
					}
				}()
			})

			checkRetrieve(t, bgCtx, rh, sh, nil)
			require.Equal(t, maxVoucherAmt, rh.ProviderNode.MaxReceivedVoucher())
		})
	}
}
