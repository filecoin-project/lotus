package modules

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-data-transfer/channelmonitor"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtnet "github.com/filecoin-project/go-data-transfer/network"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p-core/host"

	"github.com/filecoin-project/go-fil-markets/discovery"
	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/markets/storageadapter"

	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/markets"
	marketevents "github.com/filecoin-project/lotus/markets/loggers"
	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/node/repo/imports"
)

func HandleMigrateClientFunds(lc fx.Lifecycle, ds dtypes.MetadataDS, wallet full.WalletAPI, fundMgr *market.FundManager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			addr, err := wallet.WalletDefaultAddress(ctx)
			// nothing to be done if there is no default address
			if err != nil {
				return nil
			}
			b, err := ds.Get(datastore.NewKey("/marketfunds/client"))
			if err != nil {
				if xerrors.Is(err, datastore.ErrNotFound) {
					return nil
				}
				log.Errorf("client funds migration - getting datastore value: %v", err)
				return nil
			}

			var value abi.TokenAmount
			if err = value.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
				log.Errorf("client funds migration - unmarshalling datastore value: %v", err)
				return nil
			}
			_, err = fundMgr.Reserve(ctx, addr, addr, value)
			if err != nil {
				log.Errorf("client funds migration - reserving funds (wallet %s, addr %s, funds %d): %v",
					addr, addr, value, err)
				return nil
			}

			return ds.Delete(datastore.NewKey("/marketfunds/client"))
		},
	})
}

func ClientImportMgr(ds dtypes.MetadataDS, r repo.LockedRepo) (dtypes.ClientImportMgr, error) {
	// store the imports under the repo's `imports` subdirectory.
	dir := filepath.Join(r.Path(), "imports")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, xerrors.Errorf("failed to create directory %s: %w", dir, err)
	}

	ns := namespace.Wrap(ds, datastore.NewKey("/client"))
	return imports.NewManager(ns, dir), nil
}

// TODO this should be removed.
func ClientBlockstore() dtypes.ClientBlockstore {
	// in most cases this is now unused in normal operations -- however, it's important to preserve for the IPFS use case
	return blockstore.WrapIDStore(blockstore.FromDatastore(datastore.NewMapDatastore()))
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
func NewClientGraphsyncDataTransfer(lc fx.Lifecycle, h host.Host, gs dtypes.Graphsync, ds dtypes.MetadataDS, r repo.LockedRepo) (dtypes.ClientDataTransfer, error) {
	// go-data-transfer protocol retries:
	// 1s, 5s, 25s, 2m5s, 5m x 11 ~= 1 hour
	dtRetryParams := dtnet.RetryParameters(time.Second, 5*time.Minute, 15, 5)
	net := dtnet.NewFromLibp2pHost(h, dtRetryParams)

	dtDs := namespace.Wrap(ds, datastore.NewKey("/datatransfer/client/transfers"))
	transport := dtgstransport.NewTransport(h.ID(), gs, net)
	err := os.MkdirAll(filepath.Join(r.Path(), "data-transfer"), 0755) //nolint: gosec
	if err != nil && !os.IsExist(err) {
		return nil, err
	}

	// data-transfer push / pull channel restart configuration:
	dtRestartConfig := dtimpl.ChannelRestartConfig(channelmonitor.Config{
		// Disable Accept and Complete timeouts until this issue is resolved:
		// https://github.com/filecoin-project/lotus/issues/6343#
		// Wait for the other side to respond to an Open channel message
		AcceptTimeout: 0,
		// Wait for the other side to send a Complete message once all
		// data has been sent / received
		CompleteTimeout: 0,

		// When an error occurs, wait a little while until all related errors
		// have fired before sending a restart message
		RestartDebounce: 10 * time.Second,
		// After sending a restart, wait for at least 1 minute before sending another
		RestartBackoff: time.Minute,
		// After trying to restart 3 times, give up and fail the transfer
		MaxConsecutiveRestarts: 3,
	})
	dt, err := dtimpl.NewDataTransfer(dtDs, filepath.Join(r.Path(), "data-transfer"), net, transport, dtRestartConfig)
	if err != nil {
		return nil, err
	}

	dt.OnReady(marketevents.ReadyLogger("client data transfer"))
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			dt.SubscribeToEvents(marketevents.DataTransferLogger)
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

// StorageBlockstoreAccessor returns the default storage blockstore accessor
// from the import manager.
func StorageBlockstoreAccessor(importmgr dtypes.ClientImportMgr) storagemarket.BlockstoreAccessor {
	return storageadapter.NewImportsBlockstoreAccessor(importmgr)
}

// RetrievalBlockstoreAccessor returns the default retrieval blockstore accessor
// using the subdirectory `retrievals` under the repo.
func RetrievalBlockstoreAccessor(r repo.LockedRepo) (retrievalmarket.BlockstoreAccessor, error) {
	dir := filepath.Join(r.Path(), "retrievals")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, xerrors.Errorf("failed to create directory %s: %w", dir, err)
	}
	return retrievaladapter.NewCARBlockstoreAccessor(dir), nil
}

func StorageClient(lc fx.Lifecycle, h host.Host, dataTransfer dtypes.ClientDataTransfer, discovery *discoveryimpl.Local,
	deals dtypes.ClientDatastore, scn storagemarket.StorageClientNode, accessor storagemarket.BlockstoreAccessor, j journal.Journal) (storagemarket.StorageClient, error) {
	// go-fil-markets protocol retries:
	// 1s, 5s, 25s, 2m5s, 5m x 11 ~= 1 hour
	marketsRetryParams := smnet.RetryParameters(time.Second, 5*time.Minute, 15, 5)
	net := smnet.NewFromLibp2pHost(h, marketsRetryParams)

	c, err := storageimpl.NewClient(net, dataTransfer, discovery, deals, scn, accessor, storageimpl.DealPollingInterval(time.Second), storageimpl.MaxTraversalLinks(config.MaxTraversalLinks))
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
func RetrievalClient(lc fx.Lifecycle, h host.Host, r repo.LockedRepo, dt dtypes.ClientDataTransfer, payAPI payapi.PaychAPI, resolver discovery.PeerResolver,
	ds dtypes.MetadataDS, chainAPI full.ChainAPI, stateAPI full.StateAPI, accessor retrievalmarket.BlockstoreAccessor, j journal.Journal) (retrievalmarket.RetrievalClient, error) {

	adapter := retrievaladapter.NewRetrievalClientNode(payAPI, chainAPI, stateAPI)
	network := rmnet.NewFromLibp2pHost(h)
	ds = namespace.Wrap(ds, datastore.NewKey("/retrievals/client"))
	client, err := retrievalimpl.NewClient(network, dt, adapter, resolver, ds, accessor)
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
