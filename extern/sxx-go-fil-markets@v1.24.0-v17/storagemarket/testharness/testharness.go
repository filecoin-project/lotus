package testharness

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	gsnetwork "github.com/ipfs/go-graphsync/network"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	"github.com/filecoin-project/go-data-transfer/testutil"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
	"github.com/filecoin-project/go-fil-markets/stores"
)

type StorageHarness struct {
	*dependencies.StorageDependencies
	PayloadCid        cid.Cid
	Client            storagemarket.StorageClient
	Provider          storagemarket.StorageProvider
	Data              bstore.Blockstore
	ReferenceProvider *shared_testutil.MockIndexProvider
}

func NewHarness(t *testing.T, ctx context.Context, useStore bool, cd testnodes.DelayFakeCommonNode, pd testnodes.DelayFakeCommonNode,
	disableNewDeals bool, fName ...string) *StorageHarness {
	smState := testnodes.NewStorageMarketState()
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	deps := dependencies.NewDependenciesWithTestData(t, ctx, td, smState, "", cd, pd)

	return NewHarnessWithTestData(t, td, deps, useStore, disableNewDeals, fName...)
}

type MeshCreatorStub struct {
}

func (m *MeshCreatorStub) Connect(context.Context) error {
	return nil
}

func NewHarnessWithTestData(t *testing.T, td *shared_testutil.Libp2pTestData, deps *dependencies.StorageDependencies, useStore bool, disableNewDeals bool, files ...string) *StorageHarness {
	var file string
	if len(files) == 0 {
		file = "payload.txt"
	} else {
		file = files[0]
	}

	fPath := filepath.Join(shared_testutil.ThisDir(t), "../fixtures/"+file)

	var rootLink ipld.Link
	var path string
	// TODO Both functions here should return the root cid of the UnixFSDag and the carv2 file path.
	if useStore {
		rootLink, path = td.LoadUnixFSFileToStore(t, fPath)
	} else {
		rootLink, path = td.LoadUnixFSFile(t, fPath, false)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	payloadCid := rootLink.(cidlink.Link).Cid

	ba := shared_testutil.NewTestStorageBlockstoreAccessor()
	bs, err := stores.ReadOnlyFilestore(path)
	require.NoError(t, err)
	ba.Blockstore = bs
	t.Cleanup(func() { _ = bs.Close() })

	// create provider and client
	clientDs := namespace.Wrap(td.Ds1, datastore.NewKey("/deals/client"))
	client, err := storageimpl.NewClient(
		network.NewFromLibp2pHost(td.Host1, network.RetryParameters(0, 0, 0, 0)),
		deps.DTClient,
		deps.PeerResolver,
		clientDs,
		deps.ClientNode,
		ba,
		storageimpl.DealPollingInterval(0),
	)
	require.NoError(t, err)

	providerDs := namespace.Wrap(td.Ds1, datastore.NewKey("/deals/provider"))
	networkOptions := []network.Option{network.RetryParameters(0, 0, 0, 0)}
	if disableNewDeals {
		networkOptions = append(networkOptions,
			network.SupportedAskProtocols([]protocol.ID{storagemarket.OldAskProtocolID}),
			network.SupportedDealProtocols([]protocol.ID{storagemarket.DealProtocolID110}),
			network.SupportedDealStatusProtocols([]protocol.ID{storagemarket.OldDealStatusProtocolID}),
		)
	}

	rp := shared_testutil.NewMockIndexProvider()

	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2, networkOptions...),
		providerDs,
		deps.Fs,
		deps.DagStore,
		rp,
		deps.PieceStore,
		deps.DTProvider,
		deps.ProviderNode,
		deps.ProviderAddr,
		deps.StoredAsk,
		&MeshCreatorStub{},
	)
	assert.NoError(t, err)

	// set ask price where we'll accept any price
	err = provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
	assert.NoError(t, err)

	return &StorageHarness{
		StorageDependencies: deps,
		PayloadCid:          payloadCid,
		Client:              client,
		Provider:            provider,
		Data:                bs,
		ReferenceProvider:   rp,
	}
}

func (h *StorageHarness) CreateNewProvider(t *testing.T, ctx context.Context, td *shared_testutil.Libp2pTestData) storagemarket.StorageProvider {
	gs2 := graphsyncimpl.New(ctx, gsnetwork.NewFromLibp2pHost(td.Host2), td.LinkSystem2)
	dtTransport2 := dtgstransport.NewTransport(td.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(td.DTStore2, td.DTNet2, dtTransport2)
	require.NoError(t, err)
	testutil.StartAndWaitForReady(ctx, t, dt2)

	providerDs := namespace.Wrap(td.Ds1, datastore.NewKey("/deals/provider"))
	pi := shared_testutil.NewMockIndexProvider()

	provider, err := storageimpl.NewProvider(
		network.NewFromLibp2pHost(td.Host2, network.RetryParameters(0, 0, 0, 0)),
		providerDs,
		h.Fs,
		h.DagStore,
		pi,
		h.PieceStore,
		dt2,
		h.ProviderNode,
		h.ProviderAddr,
		h.StoredAsk,
		&MeshCreatorStub{},
	)
	require.NoError(t, err)
	return provider
}

func (h *StorageHarness) ProposeStorageDeal(t *testing.T, dataRef *storagemarket.DataRef, fastRetrieval, verifiedDeal bool) *storagemarket.ProposeStorageDealResult {
	var dealDuration = abi.ChainEpoch(180 * builtin.EpochsInDay)

	result, err := h.Client.ProposeStorageDeal(h.Ctx, storagemarket.ProposeStorageDealParams{
		Addr:          h.ClientAddr,
		Info:          &h.ProviderInfo,
		Data:          dataRef,
		StartEpoch:    h.Epoch + 100,
		EndEpoch:      h.Epoch + 100 + dealDuration,
		Price:         big.NewInt(1),
		Collateral:    big.NewInt(0),
		Rt:            abi.RegisteredSealProof_StackedDrg2KiBV1,
		FastRetrieval: fastRetrieval,
		VerifiedDeal:  verifiedDeal,
	})
	require.NoError(t, err)
	return result
}

func (h *StorageHarness) WaitForProviderEvent(wg *sync.WaitGroup, waitEvent storagemarket.ProviderEvent) {
	wg.Add(1)
	h.Provider.SubscribeToEvents(func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		if event == waitEvent {
			wg.Done()
		}
	})
}

func (h *StorageHarness) WaitForClientEvent(wg *sync.WaitGroup, waitEvent storagemarket.ClientEvent) {
	wg.Add(1)
	h.Client.SubscribeToEvents(func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		if event == waitEvent {
			wg.Done()
		}
	})
}
