package modules

import (
	"context"
	"time"

	"github.com/filecoin-project/go-multistore"
	"golang.org/x/xerrors"

	"go.uber.org/fx"

	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtnet "github.com/filecoin-project/go-data-transfer/network"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-fil-markets/discovery"
	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/funds"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p-core/host"

	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/markets"
	marketevents "github.com/filecoin-project/lotus/markets/loggers"
	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	"github.com/filecoin-project/lotus/node/impl/full"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/node/repo/importmgr"
	"github.com/filecoin-project/lotus/node/repo/retrievalstoremgr"
)

func ClientMultiDatastore(lc fx.Lifecycle, r repo.LockedRepo) (dtypes.ClientMultiDstore, error) {
	ds, err := r.Datastore("/client")
	if err != nil {
		return nil, xerrors.Errorf("getting datastore out of reop: %w", err)
	}

	mds, err := multistore.NewMultiDstore(ds)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return mds.Close()
		},
	})

	return mds, nil
}

func ClientImportMgr(mds dtypes.ClientMultiDstore, ds dtypes.MetadataDS) dtypes.ClientImportMgr {
	return importmgr.New(mds, namespace.Wrap(ds, datastore.NewKey("/client")))
}

func ClientBlockstore(imgr dtypes.ClientImportMgr) dtypes.ClientBlockstore {
	// in most cases this is now unused in normal operations -- however, it's important to preserve for the IPFS use case
	return blockstore.WrapIDStore(imgr.Blockstore)
}

// RegisterClientValidator is an initialization hook that registers the client
// request validator with the data transfer module as the validator for
// StorageDataTransferVoucher types
func RegisterClientValidator(crv dtypes.ClientRequestValidator, dtm dtypes.ClientDataTransfer) {
	if err := dtm.RegisterVoucherType(&requestvalidation.StorageDataTransferVoucher{}, (*requestvalidation.UnifiedRequestValidator)(crv)); err != nil {
		panic(err)
	}
}

// NewClientGraphsyncDataTransfer returns a data transfer manager that just
// uses the clients's Client DAG service for transfers
func NewClientGraphsyncDataTransfer(lc fx.Lifecycle, h host.Host, gs dtypes.Graphsync, ds dtypes.MetadataDS) (dtypes.ClientDataTransfer, error) {
	sc := storedcounter.New(ds, datastore.NewKey("/datatransfer/client/counter"))
	net := dtnet.NewFromLibp2pHost(h)

	dtDs := namespace.Wrap(ds, datastore.NewKey("/datatransfer/client/transfers"))
	transport := dtgstransport.NewTransport(h.ID(), gs)
	dt, err := dtimpl.NewDataTransfer(dtDs, net, transport, sc)
	if err != nil {
		return nil, err
	}

	dt.OnReady(marketevents.ReadyLogger("client data transfer"))
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dt.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			return dt.Stop(ctx)
		},
	})
	return dt, nil
}

// NewClientDatastore creates a datastore for the client to store its deals
func NewClientDatastore(ds dtypes.MetadataDS) dtypes.ClientDatastore {
	return namespace.Wrap(ds, datastore.NewKey("/deals/client"))
}

type ClientDealFunds funds.DealFunds

func NewClientDealFunds(ds dtypes.MetadataDS) (ClientDealFunds, error) {
	return funds.NewDealFunds(ds, datastore.NewKey("/marketfunds/client"))
}

func StorageClient(lc fx.Lifecycle, h host.Host, ibs dtypes.ClientBlockstore, mds dtypes.ClientMultiDstore, r repo.LockedRepo, dataTransfer dtypes.ClientDataTransfer, discovery *discoveryimpl.Local, deals dtypes.ClientDatastore, scn storagemarket.StorageClientNode, dealFunds ClientDealFunds, j journal.Journal) (storagemarket.StorageClient, error) {
	net := smnet.NewFromLibp2pHost(h)
	c, err := storageimpl.NewClient(net, ibs, mds, dataTransfer, discovery, deals, scn, dealFunds, storageimpl.DealPollingInterval(time.Second))
	if err != nil {
		return nil, err
	}
	c.OnReady(marketevents.ReadyLogger("storage client"))
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			c.SubscribeToEvents(marketevents.StorageClientLogger)

			evtType := j.RegisterEventType("markets/storage/client", "state_change")
			c.SubscribeToEvents(markets.StorageClientJournaler(j, evtType))

			return c.Start(ctx)
		},
		OnStop: func(context.Context) error {
			return c.Stop()
		},
	})
	return c, nil
}

// RetrievalClient creates a new retrieval client attached to the client blockstore
func RetrievalClient(lc fx.Lifecycle, h host.Host, mds dtypes.ClientMultiDstore, dt dtypes.ClientDataTransfer, payAPI payapi.PaychAPI, resolver discovery.PeerResolver, ds dtypes.MetadataDS, chainAPI full.ChainAPI, stateAPI full.StateAPI, j journal.Journal) (retrievalmarket.RetrievalClient, error) {
	adapter := retrievaladapter.NewRetrievalClientNode(payAPI, chainAPI, stateAPI)
	network := rmnet.NewFromLibp2pHost(h)
	sc := storedcounter.New(ds, datastore.NewKey("/retr"))
	client, err := retrievalimpl.NewClient(network, mds, dt, adapter, resolver, namespace.Wrap(ds, datastore.NewKey("/retrievals/client")), sc)
	if err != nil {
		return nil, err
	}
	client.OnReady(marketevents.ReadyLogger("retrieval client"))
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			client.SubscribeToEvents(marketevents.RetrievalClientLogger)

			evtType := j.RegisterEventType("markets/retrieval/client", "state_change")
			client.SubscribeToEvents(markets.RetrievalClientJournaler(j, evtType))

			return client.Start(ctx)
		},
	})
	return client, nil
}

// ClientRetrievalStoreManager is the default version of the RetrievalStoreManager that runs on multistore
func ClientRetrievalStoreManager(imgr dtypes.ClientImportMgr) dtypes.ClientRetrievalStoreManager {
	return retrievalstoremgr.NewMultiStoreRetrievalStoreManager(imgr)
}

// ClientBlockstoreRetrievalStoreManager is the default version of the RetrievalStoreManager that runs on multistore
func ClientBlockstoreRetrievalStoreManager(bs dtypes.ClientBlockstore) dtypes.ClientRetrievalStoreManager {
	return retrievalstoremgr.NewBlockstoreRetrievalStoreManager(bs)
}
