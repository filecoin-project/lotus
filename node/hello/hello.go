package hello

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"

	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborutil"
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

type NewStreamFunc func(context.Context, peer.ID, ...protocol.ID) (inet.Stream, error)
type Service struct {
	newStream NewStreamFunc

	cs     *store.ChainStore
	syncer *chain.Syncer
	pmgr   *peermgr.PeerMgr
}

func NewHelloService(h host.Host, cs *store.ChainStore, syncer *chain.Syncer, pmgr peermgr.MaybePeerMgr) *Service {
	if pmgr.Mgr == nil {
		log.Warn("running without peer manager")
	}

	return &Service{
		newStream: h.NewStream,

		cs:     cs,
		syncer: syncer,
		pmgr:   pmgr.Mgr,
	}
}

func (hs *Service) HandleStream(s inet.Stream) {
	defer s.Close()

	var hmsg Message
	if err := cborutil.ReadCborRPC(s, &hmsg); err != nil {
		log.Infow("failed to read hello message", "error", err)
		return
	}
	log.Debugw("genesis from hello",
		"tipset", hmsg.HeaviestTipSet,
		"peer", s.Conn().RemotePeer(),
		"hash", hmsg.GenesisHash)

	if hmsg.GenesisHash != hs.syncer.Genesis.Cids()[0] {
		log.Warnf("other peer has different genesis! (%s)", hmsg.GenesisHash)
		s.Conn().Close()
		return
	}
	go func() {
		if err := cborutil.WriteCborRPC(s, &Message{}); err != nil {
			log.Debugf("error while responding to latency: %v", err)
		}
	}()

	ts, err := hs.syncer.FetchTipSet(context.Background(), s.Conn().RemotePeer(), hmsg.HeaviestTipSet)
	if err != nil {
		log.Errorf("failed to fetch tipset from peer during hello: %s", err)
		return
	}

	log.Infof("Got new tipset through Hello: %s from %s", ts.Cids(), s.Conn().RemotePeer())
	hs.syncer.InformNewHead(s.Conn().RemotePeer(), ts)
	if hs.pmgr != nil {
		hs.pmgr.AddFilecoinPeer(s.Conn().RemotePeer())
	}

}

func (hs *Service) SayHello(ctx context.Context, pid peer.ID) error {
	start := time.Now()
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
	log.Info("Sending hello message: ", hts.Cids(), hts.Height(), gen.Cid())

	if err := cborutil.WriteCborRPC(s, hmsg); err != nil {
		return err
	}

	go func() {
		hmsg = &Message{}
		s.SetReadDeadline(time.Now().Add(10 * time.Second))
		_ = cborutil.ReadCborRPC(s, hmsg) // ignore error
		latency := time.Since(start)

		// add to peer tracker
		hs.pmgr.SetPeerLatency(pid, latency)
	}()

	return nil
}
