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
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/exp/rand"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	marketOld "github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestProvider_Migrations(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	deps := dependencies.NewDependenciesWithTestData(t, ctx, shared_testutil.NewLibp2pTestData(ctx, t), testnodes.NewStorageMarketState(), "",
		noOpDelay, noOpDelay)

	providerDs := namespace.Wrap(deps.TestData.Ds1, datastore.NewKey("/deals/provider"))

	numDeals := 5
	dealProposals := make([]*market.ClientDealProposal, numDeals)
	proposalCids := make([]cid.Cid, numDeals)
	addFundsCids := make([]*cid.Cid, numDeals)
	miners := make([]peer.ID, numDeals)
	clients := make([]peer.ID, numDeals)
	dealIDs := make([]abi.DealID, numDeals)
	payloadCids := make([]cid.Cid, numDeals)
	messages := make([]string, numDeals)
	publishCids := make([]*cid.Cid, numDeals)
	fastRetrievals := make([]bool, numDeals)
	storeIDs := make([]*uint64, numDeals)
	fundsReserveds := make([]abi.TokenAmount, numDeals)
	creationTimes := make([]cbg.CborTime, numDeals)
	availableForRetrievals := make([]bool, numDeals)
	piecePaths := make([]filestore.Path, numDeals)
	metadataPaths := make([]filestore.Path, numDeals)

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
		publishCids[i] = &publishMessage
		addFundsCid := shared_testutil.GenerateCids(1)[0]
		addFundsCids[i] = &addFundsCid
		dealIDs[i] = abi.DealID(rand.Uint64())
		miners[i] = shared_testutil.GeneratePeers(1)[0]
		clients[i] = shared_testutil.GeneratePeers(1)[0]
		availableForRetrievals[i] = rand.Intn(2) == 1
		piecePaths[i] = filestore.Path(shared_testutil.RandomBytes(20))
		metadataPaths[i] = filestore.Path(shared_testutil.RandomBytes(20))
		now := time.Now()
		creationTimes[i] = cbg.CborTime(time.Unix(0, now.UnixNano()).UTC())
		timeBuf := new(bytes.Buffer)
		err = creationTimes[i].MarshalCBOR(timeBuf)
		require.NoError(t, err)
		err = cborutil.ReadCborRPC(timeBuf, &creationTimes[i])
		require.NoError(t, err)
		prop, err := oldDealProposal(dealProposals[i])
		require.NoError(t, err)
		deal := migrations.MinerDeal0{
			ClientDealProposal: *prop,
			ProposalCid:        proposalCids[i],
			AddFundsCid:        addFundsCids[i],
			PublishCid:         publishCids[i],
			Miner:              miners[i],
			Client:             clients[i],
			State:              storagemarket.StorageDealExpired,
			PiecePath:          piecePaths[i],
			MetadataPath:       metadataPaths[i],
			SlashEpoch:         abi.ChainEpoch(0),
			FastRetrieval:      fastRetrievals[i],
			Message:            messages[i],
			StoreID:            storeIDs[i],
			FundsReserved:      fundsReserveds[i],
			Ref: &migrations.DataRef0{
				TransferType: storagemarket.TTGraphsync,
				Root:         payloadCids[i],
			},
			AvailableForRetrieval: availableForRetrievals[i],
			DealID:                dealIDs[i],
			CreationTime:          creationTimes[i],
		}
		buf := new(bytes.Buffer)
		err = deal.MarshalCBOR(buf)
		require.NoError(t, err)
		err = providerDs.Put(ctx, datastore.NewKey(deal.ProposalCid.String()), buf.Bytes())
		require.NoError(t, err)
	}

	pi := shared_testutil.NewMockIndexProvider()

	provider, err := storageimpl.NewProvider(

		network.NewFromLibp2pHost(deps.TestData.Host2, network.RetryParameters(0, 0, 0, 0)),
		providerDs,
		deps.Fs,
		deps.DagStore,
		pi,
		deps.PieceStore,
		deps.DTProvider,
		deps.ProviderNode,
		deps.ProviderAddr,
		deps.StoredAsk,
		&testharness.MeshCreatorStub{},
	)
	require.NoError(t, err)

	shared_testutil.StartAndWaitForReady(ctx, t, provider)
	deals, err := provider.ListLocalDeals()
	require.NoError(t, err)
	for i := 0; i < numDeals; i++ {
		var deal storagemarket.MinerDeal
		for _, testDeal := range deals {
			if testDeal.Ref.Root.Equals(payloadCids[i]) {
				deal = testDeal
				break
			}
		}
		expectedDeal := storagemarket.MinerDeal{
			ClientDealProposal: *dealProposals[i],
			ProposalCid:        proposalCids[i],
			AddFundsCid:        addFundsCids[i],
			PublishCid:         publishCids[i],
			Miner:              miners[i],
			Client:             clients[i],
			State:              storagemarket.StorageDealExpired,
			PiecePath:          piecePaths[i],
			MetadataPath:       metadataPaths[i],
			SlashEpoch:         abi.ChainEpoch(0),
			FastRetrieval:      fastRetrievals[i],
			Message:            messages[i],
			FundsReserved:      fundsReserveds[i],
			Ref: &storagemarket.DataRef{
				TransferType: storagemarket.TTGraphsync,
				Root:         payloadCids[i],
			},
			AvailableForRetrieval: availableForRetrievals[i],
			DealID:                dealIDs[i],
			CreationTime:          creationTimes[i],
		}
		require.Equal(t, expectedDeal, deal)
	}

	// Verify get deal by signed proposal cid
	deal, err := provider.GetLocalDeal(deals[0].ProposalCid)
	require.NoError(t, err)
	require.Equal(t, deals[0].ProposalCid, deal.ProposalCid)

	// Verify the deal count
	count, err := provider.LocalDealCount()
	require.NoError(t, err)
	require.Equal(t, len(deals), count)

	// Verify get a page of deals without a nil start proposal cid
	listedDeals, err := provider.ListLocalDealsPage(nil, 0, len(deals))
	require.NoError(t, err)
	require.Len(t, listedDeals, len(deals))
	for i, dl := range listedDeals {
		if i == 0 {
			continue
		}
		// Verify descending order by creation time
		require.True(t, dl.CreationTime.Time().Before(listedDeals[i-1].CreationTime.Time()))
	}
	firstDeal := listedDeals[0]
	secondDeal := listedDeals[1]
	thirdDeal := listedDeals[2]

	// Verify get a page of deals with a nil start proposal cid and with a limit
	listedDeals, err = provider.ListLocalDealsPage(nil, 0, 2)
	require.NoError(t, err)
	require.Len(t, listedDeals, 2)
	// Verify correct deals
	require.Equal(t, firstDeal.ProposalCid, listedDeals[0].ProposalCid)
	require.Equal(t, secondDeal.ProposalCid, listedDeals[1].ProposalCid)

	// Verify get a page of deals with a start proposal cid and with a limit
	listedDeals, err = provider.ListLocalDealsPage(&secondDeal.ProposalCid, 0, 2)
	require.NoError(t, err)
	require.Len(t, listedDeals, 2)
	// Verify correct deals
	require.Equal(t, secondDeal.ProposalCid, listedDeals[0].ProposalCid)
	require.Equal(t, thirdDeal.ProposalCid, listedDeals[1].ProposalCid)

	// Verify get a page of deals with a start proposal cid, and offset and a limit
	listedDeals, err = provider.ListLocalDealsPage(&secondDeal.ProposalCid, 1, 1)
	require.NoError(t, err)
	require.Len(t, listedDeals, 1)
	// Verify correct deals
	require.Equal(t, thirdDeal.ProposalCid, listedDeals[0].ProposalCid)
}

func oldDealProposal(p *market.ClientDealProposal) (*marketOld.ClientDealProposal, error) {
	label, err := p.Proposal.Label.ToString()
	if err != nil {
		return nil, err
	}
	return &marketOld.ClientDealProposal{
		Proposal: marketOld.DealProposal{
			PieceCID:             p.Proposal.PieceCID,
			PieceSize:            p.Proposal.PieceSize,
			VerifiedDeal:         p.Proposal.VerifiedDeal,
			Client:               p.Proposal.Client,
			Provider:             p.Proposal.Provider,
			Label:                label,
			StartEpoch:           p.Proposal.StartEpoch,
			EndEpoch:             p.Proposal.EndEpoch,
			StoragePricePerEpoch: p.Proposal.StoragePricePerEpoch,
			ProviderCollateral:   p.Proposal.ProviderCollateral,
			ClientCollateral:     p.Proposal.ClientCollateral,
		},
		ClientSignature: p.ClientSignature,
	}, nil
}

func TestHandleDealStream(t *testing.T) {
	t.Run("handles cases where the proposal is already being tracked", func(t *testing.T) {

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		deps := dependencies.NewDependenciesWithTestData(t, ctx, shared_testutil.NewLibp2pTestData(ctx, t), testnodes.NewStorageMarketState(), "",
			noOpDelay, noOpDelay)
		var providerDs datastore.Batching = namespace.Wrap(deps.TestData.Ds1, datastore.NewKey("/deals/provider"))
		namespaced := shared_testutil.DatastoreAtVersion(t, providerDs, "2")

		proposal := shared_testutil.MakeTestClientDealProposal()
		proposalNd, err := cborutil.AsIpld(proposal)
		require.NoError(t, err)
		payloadCid := shared_testutil.GenerateCids(1)[0]
		dataRef := &storagemarket.DataRef{
			TransferType: storagemarket.TTGraphsync,
			Root:         payloadCid,
		}

		now := time.Now()
		creationTime := cbg.CborTime(time.Unix(0, now.UnixNano()).UTC())
		timeBuf := new(bytes.Buffer)
		err = creationTime.MarshalCBOR(timeBuf)
		require.NoError(t, err)
		err = cborutil.ReadCborRPC(timeBuf, &creationTime)
		require.NoError(t, err)
		deal := storagemarket.MinerDeal{
			ClientDealProposal: *proposal,
			ProposalCid:        proposalNd.Cid(),
			State:              storagemarket.StorageDealTransferring,
			Ref:                dataRef,
		}

		// jam a miner state in
		buf := new(bytes.Buffer)
		err = deal.MarshalCBOR(buf)
		require.NoError(t, err)
		err = namespaced.Put(ctx, datastore.NewKey(deal.ProposalCid.String()), buf.Bytes())
		require.NoError(t, err)

		pi := shared_testutil.NewMockIndexProvider()

		provider, err := storageimpl.NewProvider(
			network.NewFromLibp2pHost(deps.TestData.Host2, network.RetryParameters(0, 0, 0, 0)),
			providerDs,
			deps.Fs,
			deps.DagStore,
			pi,
			deps.PieceStore,
			deps.DTProvider,
			deps.ProviderNode,
			deps.ProviderAddr,
			deps.StoredAsk,
			&testharness.MeshCreatorStub{},
		)
		require.NoError(t, err)

		impl := provider.(*storageimpl.Provider)
		shared_testutil.StartAndWaitForReady(ctx, t, impl)

		var responseWriteCount int
		s := shared_testutil.NewTestStorageDealStream(shared_testutil.TestStorageDealStreamParams{
			ProposalReader: func() (network.Proposal, error) {
				return network.Proposal{
					DealProposal:  proposal,
					Piece:         dataRef,
					FastRetrieval: false,
				}, nil
			},
			ResponseWriter: func(response network.SignedResponse, resigningFunc network.ResigningFunc) error {
				responseWriteCount += 1
				return nil
			},
		})

		// Send a deal proposal for a cid we are already tracking
		impl.HandleDealStream(s)

		require.Equal(t, 1, responseWriteCount)
	})
}
