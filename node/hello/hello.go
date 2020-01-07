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

	"github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
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

	TArrial int64
	TSent   int64
}

type NewStreamFunc func(context.Context, peer.ID, ...protocol.ID) (inet.Stream, error)
type Service struct {
	h host.Host

	cs     *store.ChainStore
	syncer *chain.Syncer
	pmgr   *peermgr.PeerMgr
}

func NewHelloService(h host.Host, cs *store.ChainStore, syncer *chain.Syncer, pmgr peermgr.MaybePeerMgr) *Service {
	if pmgr.Mgr == nil {
		log.Warn("running without peer manager")
	}

	return &Service{
		h: h,

		cs:     cs,
		syncer: syncer,
		pmgr:   pmgr.Mgr,
	}
}

func (hs *Service) HandleStream(s inet.Stream) {

	var hmsg Message
	if err := cborutil.ReadCborRPC(s, &hmsg); err != nil {
		log.Infow("failed to read hello message, diconnecting", "error", err)
		s.Conn().Close()
		return
	}
	arrived := time.Now()

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
		defer s.Close()

		sent := time.Now()
		msg := &Message{
			TArrial: arrived.UnixNano(),
			TSent:   sent.UnixNano(),
		}
		if err := cborutil.WriteCborRPC(s, msg); err != nil {
			log.Debugf("error while responding to latency: %v", err)
		}
	}()

	ts, err := hs.syncer.FetchTipSet(context.Background(), s.Conn().RemotePeer(), types.NewTipSetKey(hmsg.HeaviestTipSet...))
	if err != nil {
		log.Errorf("failed to fetch tipset from peer during hello: %s", err)
		return
	}

	if ts.TipSet().Height() > 0 {
		hs.h.ConnManager().TagPeer(s.Conn().RemotePeer(), "fcpeer", 10)

		// don't bother informing about genesis
		log.Infof("Got new tipset through Hello: %s from %s", ts.Cids(), s.Conn().RemotePeer())
		hs.syncer.InformNewHead(s.Conn().RemotePeer(), ts)
	}
	if hs.pmgr != nil {
		hs.pmgr.AddFilecoinPeer(s.Conn().RemotePeer())
	}

}

func (hs *Service) SayHello(ctx context.Context, pid peer.ID) error {
	s, err := hs.h.NewStream(ctx, pid, ProtocolID)
	if err != nil {
		return err
	}

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
	log.Debug("Sending hello message: ", hts.Cids(), hts.Height(), gen.Cid())

	t0 := time.Now()
	if err := cborutil.WriteCborRPC(s, hmsg); err != nil {
		return err
	}

	go func() {
		defer s.Close()

		hmsg = &Message{}
		s.SetReadDeadline(time.Now().Add(10 * time.Second))
		err := cborutil.ReadCborRPC(s, hmsg)
		ok := err == nil

		t3 := time.Now()
		lat := t3.Sub(t0)
		// add to peer tracker
		if hs.pmgr != nil {
			hs.pmgr.SetPeerLatency(pid, lat)
		}

		if ok {
			if hmsg.TArrial != 0 && hmsg.TSent != 0 {
				t1 := time.Unix(0, hmsg.TArrial)
				t2 := time.Unix(0, hmsg.TSent)
				offset := t0.Sub(t1) + t3.Sub(t2)
				offset /= 2
				log.Infow("time offset", "offset", offset.Seconds(), "peerid", pid.String())
			}
		}
	}()

	return nil
}
