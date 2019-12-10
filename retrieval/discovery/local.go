package discovery

import (
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
	retrievalmarket "github.com/filecoin-project/lotus/retrieval"
)

var log = logging.Logger("ret-discovery")

type Local struct {
	ds datastore.Datastore
}

func NewLocal(ds dtypes.MetadataDS) *Local {
	return &Local{ds: namespace.Wrap(ds, datastore.NewKey("/deals/local"))}
}

func (l *Local) AddPeer(cid cid.Cid, peer retrievalmarket.RetrievalPeer) error {
	// TODO: allow multiple peers here
	//  (implement an util for tracking map[thing][]otherThing, use in sectorBlockstore too)

	log.Warn("Tracking multiple retrieval peers not implemented")

	entry, err := cbor.DumpObject(peer)
	if err != nil {
		return err
	}

	return l.ds.Put(dshelp.CidToDsKey(cid), entry)
}

func (l *Local) GetPeers(data cid.Cid) ([]retrievalmarket.RetrievalPeer, error) {
	entry, err := l.ds.Get(dshelp.CidToDsKey(data))
	if err == datastore.ErrNotFound {
		return []retrievalmarket.RetrievalPeer{}, nil
	}
	if err != nil {
		return nil, err
	}
	var peer retrievalmarket.RetrievalPeer
	if err := cbor.DecodeInto(entry, &peer); err != nil {
		return nil, err
	}
	return []retrievalmarket.RetrievalPeer{peer}, nil
}

var _ retrievalmarket.PeerResolver = &Local{}
