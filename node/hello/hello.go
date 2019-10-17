package hello

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/filecoin-project/lotus/peermgr"
)

const ProtocolID = "/fil/hello/1.0.0"

var log = logging.Logger("hello")

func init() {
	cbor.RegisterCborType(Message{})
}

type Message struct {
	HeaviestTipSet       []cid.Cid
	HeaviestTipSetWeight types.BigInt
	GenesisHash          cid.Cid
}

type Service struct {
	newStream chain.NewStreamFunc

	cs     *store.ChainStore
	syncer *chain.Syncer
	pmgr   *peermgr.PeerMgr
}

func NewHelloService(h host.Host, cs *store.ChainStore, syncer *chain.Syncer, pmgr *peermgr.PeerMgr) *Service {
	return &Service{
		newStream: h.NewStream,

		cs:     cs,
		syncer: syncer,
		pmgr:   pmgr,
	}
}

func (hs *Service) HandleStream(s inet.Stream) {
	defer s.Close()

	var hmsg Message
	if err := cborrpc.ReadCborRPC(s, &hmsg); err != nil {
		log.Infow("failed to read hello message", "error", err)
		return
	}
	log.Debugw("genesis from hello",
		"tipset", hmsg.HeaviestTipSet,
		"peer", s.Conn().RemotePeer(),
		"hash", hmsg.GenesisHash)

	if hmsg.GenesisHash != hs.syncer.Genesis.Cids()[0] {
		log.Error("other peer has different genesis!")
		s.Conn().Close()
		return
	}

	ts, err := hs.syncer.FetchTipSet(context.Background(), s.Conn().RemotePeer(), hmsg.HeaviestTipSet)
	if err != nil {
		log.Errorf("failed to fetch tipset from peer during hello: %s", err)
		return
	}

	log.Infof("Got new tipset through Hello: %s from %s", ts.Cids(), s.Conn().RemotePeer())
	hs.syncer.InformNewHead(s.Conn().RemotePeer(), ts)
	hs.pmgr.AddFilecoinPeer(s.Conn().RemotePeer())
}

func (hs *Service) SayHello(ctx context.Context, pid peer.ID) error {
	s, err := hs.newStream(ctx, pid, ProtocolID)
	if err != nil {
		return err
	}
	defer s.Close()

	hts := hs.cs.GetHeaviestTipSet()
	weight, err := hs.cs.Weight(ctx, hts)
	if err != nil {
		return err
	}

	gen, err := hs.cs.GetGenesis()
	if err != nil {
		return err
	}

	hmsg := &Message{
		HeaviestTipSet:       hts.Cids(),
		HeaviestTipSetWeight: weight,
		GenesisHash:          gen.Cid(),
	}
	fmt.Println("SENDING HELLO MESSAGE: ", hts.Cids(), hts.Height())
	fmt.Println("hello message genesis: ", gen.Cid())

	if err := cborrpc.WriteCborRPC(s, hmsg); err != nil {
		return err
	}

	return nil
}
