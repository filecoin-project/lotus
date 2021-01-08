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
	"github.com/filecoin-project/lotus/node/config"
)

func TestDealPublisher(t *testing.T) {
	testCases := []struct {
		name                         string
		publishPeriod                time.Duration
		maxDealsPerMsg               uint64
		dealCountWithinPublishPeriod int
		expiredWithinPublishPeriod   int
		dealCountAfterPublishPeriod  int
		expectedDealsPerMsg          []int
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
		name:                         "ignore expired deals",
		publishPeriod:                10 * time.Millisecond,
		maxDealsPerMsg:               5,
		dealCountWithinPublishPeriod: 2,
		expiredWithinPublishPeriod:   2,
		dealCountAfterPublishPeriod:  1,
		expectedDealsPerMsg:          []int{2, 1},
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := tutils.NewActorAddr(t, "client")
			provider := tutils.NewActorAddr(t, "provider")
			worker := tutils.NewActorAddr(t, "worker")
			dpapi := newDPAPI(t, worker)

			// Create a deal publisher
			dp := newDealPublisher(dpapi, &config.PublishMsgConfig{
				PublishPeriod:  config.Duration(tc.publishPeriod),
				MaxDealsPerMsg: tc.maxDealsPerMsg,
			}, &api.MessageSendSpec{MaxFee: abi.NewTokenAmount(1)})

			// Keep a record of the deals that were submitted to be published
			var dealsToPublish []market.ClientDealProposal
			publishDeal := func(expired bool) {
				pctx := ctx
				var cancel context.CancelFunc
				if expired {
					pctx, cancel = context.WithCancel(ctx)
					cancel()
				}

				deal := market.ClientDealProposal{
					Proposal: market0.DealProposal{
						PieceCID: generateCids(1)[0],
						Client:   client,
						Provider: provider,
					},
					ClientSignature: crypto.Signature{
						Type: crypto.SigTypeSecp256k1,
						Data: []byte("signature data"),
					},
				}
				if !expired {
					dealsToPublish = append(dealsToPublish, deal)
				}
				go func() {
					_, err := dp.Publish(pctx, deal)
					if expired {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				}()
			}

			// Publish deals within publish period
			for i := 0; i < tc.dealCountWithinPublishPeriod; i++ {
				publishDeal(false)
			}
			for i := 0; i < tc.expiredWithinPublishPeriod; i++ {
				publishDeal(true)
			}

			// Wait until publish period has elapsed
			time.Sleep(2 * tc.publishPeriod)

			// Publish deals after publish period
			for i := 0; i < tc.dealCountAfterPublishPeriod; i++ {
				publishDeal(false)
			}

			// For each message that was expected to be sent
			var publishedDeals []market.ClientDealProposal
			for _, expectedDealsInMsg := range tc.expectedDealsPerMsg {
				// Should have called StateMinerInfo with the provider address
				stateMinerInfoAddr := <-dpapi.stateMinerInfoCalls
				require.Equal(t, provider, stateMinerInfoAddr)

				// Check the fields of the message that was sent
				msg := <-dpapi.pushedMsgs
				require.Equal(t, worker, msg.From)
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
		})
	}
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

func newDPAPI(t *testing.T, worker address.Address) *dpAPI {
	return &dpAPI{
		t:                   t,
		worker:              worker,
		stateMinerInfoCalls: make(chan address.Address, 128),
		pushedMsgs:          make(chan *types.Message, 128),
	}
}

func (d *dpAPI) StateMinerInfo(ctx context.Context, address address.Address, key types.TipSetKey) (miner.MinerInfo, error) {
	d.stateMinerInfoCalls <- address
	return miner.MinerInfo{Worker: d.worker}, nil
}

func (d *dpAPI) MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	d.pushedMsgs <- msg
	return &types.SignedMessage{Message: *msg}, nil
}
