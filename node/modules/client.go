package modules

import (
	"context"
	"path/filepath"
	"reflect"

	"github.com/filecoin-project/go-data-transfer/impl/graphsync"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	deals "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-statestore"
	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-filestore"
	graphsync "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/ipldbridge"
	gsnet "github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-graphsync/storeutil"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-merkledag"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/routing"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/paych"
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
func RegisterClientValidator(crv *deals.ClientRequestValidator, dtm dtypes.ClientDataTransfer) {
	if err := dtm.RegisterVoucherType(reflect.TypeOf(&deals.StorageDataTransferVoucher{}), crv); err != nil {
		panic(err)
	}
}

// NewClientDAGServiceDataTransfer returns a data transfer manager that just
// uses the clients's Client DAG service for transfers
func NewClientDAGServiceDataTransfer(h host.Host, gs dtypes.ClientGraphsync) dtypes.ClientDataTransfer {
	return graphsyncimpl.NewGraphSyncDataTransfer(h, gs)
}

// NewClientDealStore creates a statestore for the client to store its deals
func NewClientDealStore(ds dtypes.MetadataDS) dtypes.ClientDealStore {
	return statestore.New(namespace.Wrap(ds, datastore.NewKey("/deals/client")))
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

// ClientGraphsync creates a graphsync instance which reads and writes blocks
// to the ClientBlockstore
func ClientGraphsync(mctx helpers.MetricsCtx, lc fx.Lifecycle, ibs dtypes.ClientBlockstore, h host.Host) dtypes.ClientGraphsync {
	graphsyncNetwork := gsnet.NewFromLibp2pHost(h)
	ipldBridge := ipldbridge.NewIPLDBridge()
	loader := storeutil.LoaderForBlockstore(ibs)
	storer := storeutil.StorerForBlockstore(ibs)
	gs := graphsync.New(helpers.LifecycleCtx(mctx, lc), graphsyncNetwork, ipldBridge, loader, storer)

	return gs
}

func NewClientRequestValidator(deals dtypes.ClientDealStore) *storageimpl.ClientRequestValidator {
	return storageimpl.NewClientRequestValidator(deals)
}

func StorageClient(h host.Host, dag dtypes.ClientDAG, dataTransfer dtypes.ClientDataTransfer, discovery *discovery.Local, deals dtypes.ClientDealStore, scn storagemarket.StorageClientNode) storagemarket.StorageClient {
	return storageimpl.NewClient(h, dag, dataTransfer, discovery, deals, scn)
}

// RetrievalClient creates a new retrieval client attached to the client blockstore
func RetrievalClient(h host.Host, bs dtypes.ClientBlockstore, pmgr *paych.Manager, payapi payapi.PaychAPI) retrievalmarket.RetrievalClient {
	adapter := retrievaladapter.NewRetrievalClientNode(pmgr, payapi)
	return retrievalimpl.NewClient(h, bs, adapter)
}
