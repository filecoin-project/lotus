package discoveryimpl

import (
	"bytes"
	"context"

	"github.com/hannahhoward/go-pubsub"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	logging "github.com/ipfs/go-log/v2"

	cborutil "github.com/filecoin-project/go-cbor-util"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	versionedds "github.com/filecoin-project/go-ds-versioning/pkg/datastore"

	"github.com/filecoin-project/go-fil-markets/discovery"
	"github.com/filecoin-project/go-fil-markets/discovery/migrations"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var log = logging.Logger("retrieval-discovery")

type Local struct {
	ds        datastore.Datastore
	migrateDs func(context.Context) error
	readySub  *pubsub.PubSub
}

func NewLocal(ds datastore.Batching) (*Local, error) {
	migrations, err := migrations.RetrievalPeersMigrations.Build()
	if err != nil {
		return nil, err
	}
	versionedDs, migrateDs := versionedds.NewVersionedDatastore(ds, migrations, versioning.VersionKey("1"))
	readySub := pubsub.New(shared.ReadyDispatcher)
	return &Local{versionedDs, migrateDs, readySub}, nil
}

func (l *Local) Start(ctx context.Context) error {
	go func() {
		err := l.migrateDs(ctx)
		if err != nil {
			log.Errorf("Migrating retrieval peers: %s", err.Error())
		}
		err = l.readySub.Publish(err)
		if err != nil {
			log.Warnf("Publishing retrieval peers list ready event: %s", err.Error())
		}
	}()
	return nil
}

// OnReady registers a listener for when the retrieval peers list has finished starting up
func (l *Local) OnReady(ready shared.ReadyFunc) {
	l.readySub.Subscribe(ready)
}

func (l *Local) AddPeer(ctx context.Context, cid cid.Cid, peer retrievalmarket.RetrievalPeer) error {
	key := dshelp.MultihashToDsKey(cid.Hash())
	exists, err := l.ds.Has(ctx, key)
	if err != nil {
		return err
	}

	var newRecord bytes.Buffer

	if !exists {
		peers := discovery.RetrievalPeers{Peers: []retrievalmarket.RetrievalPeer{peer}}
		err = cborutil.WriteCborRPC(&newRecord, &peers)
		if err != nil {
			return err
		}
	} else {
		entry, err := l.ds.Get(ctx, key)
		if err != nil {
			return err
		}
		var peers discovery.RetrievalPeers
		if err = cborutil.ReadCborRPC(bytes.NewReader(entry), &peers); err != nil {
			return err
		}
		if hasPeer(peers, peer) {
			return nil
		}
		peers.Peers = append(peers.Peers, peer)
		err = cborutil.WriteCborRPC(&newRecord, &peers)
		if err != nil {
			return err
		}
	}

	return l.ds.Put(ctx, key, newRecord.Bytes())
}

func hasPeer(peerList discovery.RetrievalPeers, peer retrievalmarket.RetrievalPeer) bool {
	for _, p := range peerList.Peers {
		if p == peer {
			return true
		}
	}
	return false
}

func (l *Local) GetPeers(payloadCID cid.Cid) ([]retrievalmarket.RetrievalPeer, error) {
	entry, err := l.ds.Get(context.TODO(), dshelp.MultihashToDsKey(payloadCID.Hash()))
	if err == datastore.ErrNotFound {
		return []retrievalmarket.RetrievalPeer{}, nil
	}
	if err != nil {
		return nil, err
	}
	var peers discovery.RetrievalPeers
	if err := cborutil.ReadCborRPC(bytes.NewReader(entry), &peers); err != nil {
		return nil, err
	}
	return peers.Peers, nil
}

var _ discovery.PeerResolver = &Local{}
