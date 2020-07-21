package modules

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/lib/bufbstore"
	"golang.org/x/xerrors"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p-core/host"
	"go.uber.org/fx"

	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtnet "github.com/filecoin-project/go-data-transfer/network"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"

	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	"github.com/filecoin-project/lotus/node/impl/full"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/node/repo/importmgr"
	"github.com/filecoin-project/lotus/paychmgr"
)

func ClientMultiDatastore(lc fx.Lifecycle, r repo.LockedRepo) (dtypes.ClientMultiDstore, error) {
	ds, err := r.Datastore("/client")
	if err != nil {
		return nil, xerrors.Errorf("getting datastore out of reop: %w", err)
	}

	mds, err := importmgr.NewMultiDstore(ds)
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
	// TODO: This isn't.. the best
	//  - If it's easy to pass per-retrieval blockstores with markets we don't need this
	//  - If it's not easy, we need to store this in a separate datastore on disk
	defaultWrite := blockstore.NewBlockstore(datastore.NewMapDatastore())

	return blockstore.NewIdStore(bufbstore.NewTieredBstore(imgr.Blockstore, defaultWrite))
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

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dt.Start(ctx)
		},
		OnStop: func(context.Context) error {
			return dt.Stop()
		},
	})
	return dt, nil
}

// NewClientDealStore creates a statestore for the client to store its deals
func NewClientDealStore(ds dtypes.ClientDatastore) dtypes.ClientDealStore {
	return statestore.New(ds)
}

// NewClientDatastore creates a datastore for the client to store its deals
func NewClientDatastore(ds dtypes.MetadataDS) dtypes.ClientDatastore {
	return namespace.Wrap(ds, datastore.NewKey("/deals/client"))
}

func NewClientRequestValidator(deals dtypes.ClientDealStore) dtypes.ClientRequestValidator {
	return requestvalidation.NewUnifiedRequestValidator(nil, deals)
}

func StorageClient(lc fx.Lifecycle, h host.Host, ibs dtypes.ClientBlockstore, r repo.LockedRepo, dataTransfer dtypes.ClientDataTransfer, discovery *discovery.Local, deals dtypes.ClientDatastore, scn storagemarket.StorageClientNode) (storagemarket.StorageClient, error) {
	net := smnet.NewFromLibp2pHost(h)
	c, err := storageimpl.NewClient(net, ibs, dataTransfer, discovery, deals, scn, storageimpl.DealPollingInterval(time.Second))
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return c.Start(ctx)
		},
		OnStop: func(context.Context) error {
			c.Stop()
			return nil
		},
	})
	return c, nil
}

// RetrievalClient creates a new retrieval client attached to the client blockstore
func RetrievalClient(h host.Host, bs dtypes.ClientBlockstore, dt dtypes.ClientDataTransfer, pmgr *paychmgr.Manager, payapi payapi.PaychAPI, resolver retrievalmarket.PeerResolver, ds dtypes.MetadataDS, chainapi full.ChainAPI) (retrievalmarket.RetrievalClient, error) {
	adapter := retrievaladapter.NewRetrievalClientNode(pmgr, payapi, chainapi)
	network := rmnet.NewFromLibp2pHost(h)
	sc := storedcounter.New(ds, datastore.NewKey("/retr"))
	return retrievalimpl.NewClient(network, bs, dt, adapter, resolver, namespace.Wrap(ds, datastore.NewKey("/retrievals/client")), sc)
}
