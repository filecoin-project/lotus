package storageimpl_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/exp/rand"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

var noOpDelay = testnodes.DelayFakeCommonNode{}

func TestClient_Configure(t *testing.T) {
	c := &storageimpl.Client{}
	assert.Equal(t, time.Duration(0), c.PollingInterval())

	c.Configure(storageimpl.DealPollingInterval(123 * time.Second))

	assert.Equal(t, 123*time.Second, c.PollingInterval())
}

func TestClient_Migrations(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	deps := dependencies.NewDependenciesWithTestData(t, ctx, shared_testutil.NewLibp2pTestData(ctx, t), testnodes.NewStorageMarketState(), "", noOpDelay,
		noOpDelay)

	clientDs := namespace.Wrap(deps.TestData.Ds1, datastore.NewKey("/deals/client"))

	numDeals := 5
	dealProposals := make([]*market.ClientDealProposal, numDeals)
	proposalCids := make([]cid.Cid, numDeals)
	addFundsCids := make([]*cid.Cid, numDeals)
	miners := make([]peer.ID, numDeals)
	dealIDs := make([]abi.DealID, numDeals)
	payloadCids := make([]cid.Cid, numDeals)
	messages := make([]string, numDeals)
	publishMessages := make([]*cid.Cid, numDeals)
	fastRetrievals := make([]bool, numDeals)
	storeIDs := make([]*uint64, numDeals)
	fundsReserveds := make([]abi.TokenAmount, numDeals)
	creationTimes := make([]cbg.CborTime, numDeals)

	for i := 0; i < numDeals; i++ {
		dealProposals[i] = shared_testutil.MakeTestClientDealProposal()
		proposalNd, err := cborutil.AsIpld(dealProposals[i])
		require.NoError(t, err)
		proposalCids[i] = proposalNd.Cid()
		payloadCids[i] = shared_testutil.GenerateCids(1)[0]
		storeID := rand.Uint64()
		storeIDs[i] = &storeID
		messages[i] = string(shared_testutil.RandomBytes(20))
		fundsReserveds[i] = big.NewInt(rand.Int63())
		fastRetrievals[i] = rand.Intn(2) == 1
		publishMessage := shared_testutil.GenerateCids(1)[0]
		publishMessages[i] = &publishMessage
		addFundsCid := shared_testutil.GenerateCids(1)[0]
		addFundsCids[i] = &addFundsCid
		dealIDs[i] = abi.DealID(rand.Uint64())
		miners[i] = shared_testutil.GeneratePeers(1)[0]
		now := time.Now()
		creationTimes[i] = cbg.CborTime(time.Unix(0, now.UnixNano()).UTC())
		timeBuf := new(bytes.Buffer)
		err = creationTimes[i].MarshalCBOR(timeBuf)
		require.NoError(t, err)
		err = cborutil.ReadCborRPC(timeBuf, &creationTimes[i])
		require.NoError(t, err)
		deal := migrations.ClientDeal0{
			ClientDealProposal: *dealProposals[i],
			ProposalCid:        proposalCids[i],
			AddFundsCid:        addFundsCids[i],
			State:              storagemarket.StorageDealExpired,
			Miner:              miners[i],
			MinerWorker:        address.TestAddress2,
			DealID:             dealIDs[i],
			DataRef: &migrations.DataRef0{
				TransferType: storagemarket.TTGraphsync,
				Root:         payloadCids[i],
			},
			Message:        messages[i],
			PublishMessage: publishMessages[i],
			SlashEpoch:     abi.ChainEpoch(0),
			PollRetryCount: 0,
			PollErrorCount: 0,
			FastRetrieval:  fastRetrievals[i],
			StoreID:        storeIDs[i],
			FundsReserved:  fundsReserveds[i],
			CreationTime:   creationTimes[i],
		}
		buf := new(bytes.Buffer)
		err = deal.MarshalCBOR(buf)
		require.NoError(t, err)
		err = clientDs.Put(ctx, datastore.NewKey(deal.ProposalCid.String()), buf.Bytes())
		require.NoError(t, err)
	}
	client, err := storageimpl.NewClient(
		network.NewFromLibp2pHost(deps.TestData.Host1, network.RetryParameters(0, 0, 0, 0)),
		deps.DTClient,
		deps.PeerResolver,
		clientDs,
		deps.ClientNode,
		shared_testutil.NewTestStorageBlockstoreAccessor(),
		storageimpl.DealPollingInterval(0),
	)
	require.NoError(t, err)

	shared_testutil.StartAndWaitForReady(ctx, t, client)
	deals, err := client.ListLocalDeals(ctx)
	require.NoError(t, err)
	for i := 0; i < numDeals; i++ {
		var deal storagemarket.ClientDeal
		for _, testDeal := range deals {
			if testDeal.DataRef.Root.Equals(payloadCids[i]) {
				deal = testDeal
				break
			}
		}
		expectedDeal := storagemarket.ClientDeal{
			ClientDealProposal: *dealProposals[i],
			ProposalCid:        proposalCids[i],
			AddFundsCid:        addFundsCids[i],
			State:              storagemarket.StorageDealExpired,
			Miner:              miners[i],
			MinerWorker:        address.TestAddress2,
			DealID:             dealIDs[i],
			DataRef: &storagemarket.DataRef{
				TransferType: storagemarket.TTGraphsync,
				Root:         payloadCids[i],
			},
			Message:        messages[i],
			PublishMessage: publishMessages[i],
			SlashEpoch:     abi.ChainEpoch(0),
			PollRetryCount: 0,
			PollErrorCount: 0,
			FastRetrieval:  fastRetrievals[i],
			FundsReserved:  fundsReserveds[i],
			CreationTime:   creationTimes[i],
		}
		require.Equal(t, expectedDeal, deal)
	}
}
