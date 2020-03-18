package modules

import (
	"context"
	"path/filepath"
	"reflect"

	graphsyncimpl "github.com/filecoin-project/go-data-transfer/impl/graphsync"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storedcounter"
	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-filestore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-merkledag"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/routing"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	"github.com/filecoin-project/lotus/node/impl/full"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/paychmgr"
)

func ClientFstore(r repo.LockedRepo) (dtypes.ClientFilestore, error) {
	clientds, err := r.Datastore("/client")
	if err != nil {
		return nil, err
	}
	blocks := namespace.Wrap(clientds, datastore.NewKey("blocks"))

	fm := filestore.NewFileManager(clientds, filepath.Dir(r.Path()))
	fm.AllowFiles = true
	// TODO: fm.AllowUrls (needs more code in client import)

	bs := blockstore.NewBlockstore(blocks)
	return filestore.NewFilestore(bs, fm), nil
}

func ClientBlockstore(fstore dtypes.ClientFilestore) dtypes.ClientBlockstore {
	return blockstore.NewIdStore((*filestore.Filestore)(fstore))
}

// RegisterClientValidator is an initialization hook that registers the client
// request validator with the data transfer module as the validator for
// StorageDataTransferVoucher types
func RegisterClientValidator(crv *requestvalidation.ClientRequestValidator, dtm dtypes.ClientDataTransfer) {
	if err := dtm.RegisterVoucherType(reflect.TypeOf(&requestvalidation.StorageDataTransferVoucher{}), crv); err != nil {
		panic(err)
	}
}

// NewClientGraphsyncDataTransfer returns a data transfer manager that just
// uses the clients's Client DAG service for transfers
func NewClientGraphsyncDataTransfer(h host.Host, gs dtypes.Graphsync) dtypes.ClientDataTransfer {
	return graphsyncimpl.NewGraphSyncDataTransfer(h, gs)
}

// NewClientDealStore creates a statestore for the client to store its deals
func NewClientDealStore(ds dtypes.ClientDatastore) dtypes.ClientDealStore {
	return statestore.New(ds)
}

// NewClientDatastore creates a datastore for the client to store its deals
func NewClientDatastore(ds dtypes.MetadataDS) dtypes.ClientDatastore {
	return namespace.Wrap(ds, datastore.NewKey("/deals/client"))
}

// ClientDAG is a DAGService for the ClientBlockstore
func ClientDAG(mctx helpers.MetricsCtx, lc fx.Lifecycle, ibs dtypes.ClientBlockstore, rt routing.Routing, h host.Host) dtypes.ClientDAG {
	bitswapNetwork := network.NewFromIpfsHost(h, rt)
	exch := bitswap.New(helpers.LifecycleCtx(mctx, lc), bitswapNetwork, ibs)

	bsvc := blockservice.New(ibs, exch)
	dag := merkledag.NewDAGService(bsvc)

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return bsvc.Close()
		},
	})

	return dag
}

func NewClientRequestValidator(deals dtypes.ClientDealStore) *requestvalidation.ClientRequestValidator {
	return requestvalidation.NewClientRequestValidator(deals)
}

func StorageClient(h host.Host, ibs dtypes.ClientBlockstore, r repo.LockedRepo, dataTransfer dtypes.ClientDataTransfer, discovery *discovery.Local, deals dtypes.ClientDatastore, scn storagemarket.StorageClientNode) (storagemarket.StorageClient, error) {
	net := smnet.NewFromLibp2pHost(h)
	return storageimpl.NewClient(net, ibs, dataTransfer, discovery, deals, scn)
}

// RetrievalClient creates a new retrieval client attached to the client blockstore
func RetrievalClient(h host.Host, bs dtypes.ClientBlockstore, pmgr *paychmgr.Manager, payapi payapi.PaychAPI, resolver retrievalmarket.PeerResolver, ds dtypes.MetadataDS, chainapi full.ChainAPI) (retrievalmarket.RetrievalClient, error) {
	adapter := retrievaladapter.NewRetrievalClientNode(pmgr, payapi, chainapi)
	network := rmnet.NewFromLibp2pHost(h)
	sc := storedcounter.New(ds, datastore.NewKey("/retr"))
	return retrievalimpl.NewClient(network, bs, adapter, resolver, ds, sc)
}
