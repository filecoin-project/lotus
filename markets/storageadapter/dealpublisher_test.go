package storageadapter

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/crypto"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	"github.com/ipfs/go-cid"

	"github.com/stretchr/testify/require"

	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
)

func TestDealPublisher(t *testing.T) {
	testCases := []struct {
		name                            string
		publishPeriod                   time.Duration
		maxDealsPerMsg                  uint64
		dealCountWithinPublishPeriod    int
		ctxCancelledWithinPublishPeriod int
		expiredDeals                    int
		dealCountAfterPublishPeriod     int
		expectedDealsPerMsg             []int
	}{{
		name:                         "publish one deal within publish period",
		publishPeriod:                10 * time.Millisecond,
		maxDealsPerMsg:               5,
		dealCountWithinPublishPeriod: 1,
		dealCountAfterPublishPeriod:  0,
		expectedDealsPerMsg:          []int{1},
	}, {
		name:                         "publish two deals within publish period",
		publishPeriod:                10 * time.Millisecond,
		maxDealsPerMsg:               5,
		dealCountWithinPublishPeriod: 2,
		dealCountAfterPublishPeriod:  0,
		expectedDealsPerMsg:          []int{2},
	}, {
		name:                         "publish one deal within publish period, and one after",
		publishPeriod:                10 * time.Millisecond,
		maxDealsPerMsg:               5,
		dealCountWithinPublishPeriod: 1,
		dealCountAfterPublishPeriod:  1,
		expectedDealsPerMsg:          []int{1, 1},
	}, {
		name:                         "publish deals that exceed max deals per message within publish period, and one after",
		publishPeriod:                10 * time.Millisecond,
		maxDealsPerMsg:               2,
		dealCountWithinPublishPeriod: 3,
		dealCountAfterPublishPeriod:  1,
		expectedDealsPerMsg:          []int{2, 1, 1},
	}, {
		name:                            "ignore deals with cancelled context",
		publishPeriod:                   10 * time.Millisecond,
		maxDealsPerMsg:                  5,
		dealCountWithinPublishPeriod:    2,
		ctxCancelledWithinPublishPeriod: 2,
		dealCountAfterPublishPeriod:     1,
		expectedDealsPerMsg:             []int{2, 1},
	}, {
		name:                         "ignore expired deals",
		publishPeriod:                10 * time.Millisecond,
		maxDealsPerMsg:               5,
		dealCountWithinPublishPeriod: 2,
		expiredDeals:                 2,
		dealCountAfterPublishPeriod:  1,
		expectedDealsPerMsg:          []int{2, 1},
	}, {
		name:                            "zero config",
		publishPeriod:                   0,
		maxDealsPerMsg:                  0,
		dealCountWithinPublishPeriod:    2,
		ctxCancelledWithinPublishPeriod: 0,
		dealCountAfterPublishPeriod:     2,
		expectedDealsPerMsg:             []int{1, 1, 1, 1},
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dpapi := newDPAPI(t)

			// Create a deal publisher
			dp := newDealPublisher(dpapi, PublishMsgConfig{
				Period:         tc.publishPeriod,
				MaxDealsPerMsg: tc.maxDealsPerMsg,
			}, &api.MessageSendSpec{MaxFee: abi.NewTokenAmount(1)})

			// Keep a record of the deals that were submitted to be published
			var dealsToPublish []market.ClientDealProposal

			// Publish deals within publish period
			for i := 0; i < tc.dealCountWithinPublishPeriod; i++ {
				deal := publishDeal(t, dp, false, false)
				dealsToPublish = append(dealsToPublish, deal)
			}
			for i := 0; i < tc.ctxCancelledWithinPublishPeriod; i++ {
				publishDeal(t, dp, true, false)
			}
			for i := 0; i < tc.expiredDeals; i++ {
				publishDeal(t, dp, false, true)
			}

			// Wait until publish period has elapsed
			time.Sleep(2 * tc.publishPeriod)

			// Publish deals after publish period
			for i := 0; i < tc.dealCountAfterPublishPeriod; i++ {
				deal := publishDeal(t, dp, false, false)
				dealsToPublish = append(dealsToPublish, deal)
			}

			checkPublishedDeals(t, dpapi, dealsToPublish, tc.expectedDealsPerMsg)
		})
	}
}

func TestForcePublish(t *testing.T) {
	dpapi := newDPAPI(t)

	// Create a deal publisher
	start := time.Now()
	publishPeriod := time.Hour
	dp := newDealPublisher(dpapi, PublishMsgConfig{
		Period:         publishPeriod,
		MaxDealsPerMsg: 10,
	}, &api.MessageSendSpec{MaxFee: abi.NewTokenAmount(1)})

	// Queue three deals for publishing, one with a cancelled context
	var dealsToPublish []market.ClientDealProposal
	// 1. Regular deal
	deal := publishDeal(t, dp, false, false)
	dealsToPublish = append(dealsToPublish, deal)
	// 2. Deal with cancelled context
	publishDeal(t, dp, true, false)
	// 3. Regular deal
	deal = publishDeal(t, dp, false, false)
	dealsToPublish = append(dealsToPublish, deal)

	// Allow a moment for them to be queued
	time.Sleep(10 * time.Millisecond)

	// Should be two deals in the pending deals list
	// (deal with cancelled context is ignored)
	pendingInfo := dp.PendingDeals()
	require.Len(t, pendingInfo.Deals, 2)
	require.Equal(t, publishPeriod, pendingInfo.PublishPeriod)
	require.True(t, pendingInfo.PublishPeriodStart.After(start))
	require.True(t, pendingInfo.PublishPeriodStart.Before(time.Now()))

	// Force publish all pending deals
	dp.ForcePublishPendingDeals()

	// Should be no pending deals
	pendingInfo = dp.PendingDeals()
	require.Len(t, pendingInfo.Deals, 0)

	// Make sure the expected deals were published
	checkPublishedDeals(t, dpapi, dealsToPublish, []int{2})
}

func publishDeal(t *testing.T, dp *DealPublisher, ctxCancelled bool, expired bool) market.ClientDealProposal {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pctx := ctx
	if ctxCancelled {
		pctx, cancel = context.WithCancel(ctx)
		cancel()
	}

	startEpoch := abi.ChainEpoch(20)
	if expired {
		startEpoch = abi.ChainEpoch(5)
	}
	deal := market.ClientDealProposal{
		Proposal: market0.DealProposal{
			PieceCID:   generateCids(1)[0],
			Client:     getClientActor(t),
			Provider:   getProviderActor(t),
			StartEpoch: startEpoch,
			EndEpoch:   abi.ChainEpoch(120),
		},
		ClientSignature: crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: []byte("signature data"),
		},
	}

	go func() {
		_, err := dp.Publish(pctx, deal)

		// If the test has completed just bail out without checking for errors
		if ctx.Err() != nil {
			return
		}

		if ctxCancelled || expired {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}()

	return deal
}

func checkPublishedDeals(t *testing.T, dpapi *dpAPI, dealsToPublish []market.ClientDealProposal, expectedDealsPerMsg []int) {
	// For each message that was expected to be sent
	var publishedDeals []market.ClientDealProposal
	for _, expectedDealsInMsg := range expectedDealsPerMsg {
		// Should have called StateMinerInfo with the provider address
		stateMinerInfoAddr := <-dpapi.stateMinerInfoCalls
		require.Equal(t, getProviderActor(t), stateMinerInfoAddr)

		// Check the fields of the message that was sent
		msg := <-dpapi.pushedMsgs
		require.Equal(t, getWorkerActor(t), msg.From)
		require.Equal(t, market.Address, msg.To)
		require.Equal(t, market.Methods.PublishStorageDeals, msg.Method)

		// Check that the expected number of deals was included in the message
		var params market2.PublishStorageDealsParams
		err := params.UnmarshalCBOR(bytes.NewReader(msg.Params))
		require.NoError(t, err)
		require.Len(t, params.Deals, expectedDealsInMsg)

		// Keep track of the deals that were sent
		for _, d := range params.Deals {
			publishedDeals = append(publishedDeals, d)
		}
	}

	// Verify that all deals that were submitted to be published were
	// sent out (we do this by ensuring all the piece CIDs are present)
	require.True(t, matchPieceCids(publishedDeals, dealsToPublish))
}

func matchPieceCids(sent []market.ClientDealProposal, exp []market.ClientDealProposal) bool {
	cidsA := dealPieceCids(sent)
	cidsB := dealPieceCids(exp)

	if len(cidsA) != len(cidsB) {
		return false
	}

	s1 := cid.NewSet()
	for _, c := range cidsA {
		s1.Add(c)
	}

	for _, c := range cidsB {
		if !s1.Has(c) {
			return false
		}
	}

	return true
}

func dealPieceCids(deals []market2.ClientDealProposal) []cid.Cid {
	cids := make([]cid.Cid, 0, len(deals))
	for _, dl := range deals {
		cids = append(cids, dl.Proposal.PieceCID)
	}
	return cids
}

type dpAPI struct {
	t      *testing.T
	worker address.Address

	stateMinerInfoCalls chan address.Address
	pushedMsgs          chan *types.Message
}

func newDPAPI(t *testing.T) *dpAPI {
	return &dpAPI{
		t:                   t,
		worker:              getWorkerActor(t),
		stateMinerInfoCalls: make(chan address.Address, 128),
		pushedMsgs:          make(chan *types.Message, 128),
	}
}

func (d *dpAPI) ChainHead(ctx context.Context) (*types.TipSet, error) {
	dummyCid, err := cid.Parse("bafkqaaa")
	require.NoError(d.t, err)
	return types.NewTipSet([]*types.BlockHeader{{
		Miner:                 tutils.NewActorAddr(d.t, "miner"),
		Height:                abi.ChainEpoch(10),
		ParentStateRoot:       dummyCid,
		Messages:              dummyCid,
		ParentMessageReceipts: dummyCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS},
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS},
	}})
}

func (d *dpAPI) StateMinerInfo(ctx context.Context, address address.Address, key types.TipSetKey) (miner.MinerInfo, error) {
	d.stateMinerInfoCalls <- address
	return miner.MinerInfo{Worker: d.worker}, nil
}

func (d *dpAPI) MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	d.pushedMsgs <- msg
	return &types.SignedMessage{Message: *msg}, nil
}

func getClientActor(t *testing.T) address.Address {
	return tutils.NewActorAddr(t, "client")
}

func getWorkerActor(t *testing.T) address.Address {
	return tutils.NewActorAddr(t, "worker")
}

func getProviderActor(t *testing.T) address.Address {
	return tutils.NewActorAddr(t, "provider")
}
