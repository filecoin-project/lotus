package hello

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	inet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"golang.org/x/xerrors"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/peermgr"
)

// TODO(TEST): missing test coverage.

const ProtocolID = "/fil/hello/1.0.0"

var log = logging.Logger("hello")
var streamDeadline = 10 * time.Second
var streamOpenTimeout = 1 * time.Minute

type HelloMessage struct {
	HeaviestTipSet       []cid.Cid
	HeaviestTipSetHeight abi.ChainEpoch
	HeaviestTipSetWeight big.Int
	GenesisHash          cid.Cid
}

type LatencyMessage struct {
	TArrival int64
	TSent    int64
}

type NewStreamFunc func(context.Context, peer.ID, ...protocol.ID) (inet.Stream, error)

type Service struct {
	h host.Host

	cs     *store.ChainStore
	syncer *chain.Syncer
	cons   consensus.Consensus
	pmgr   *peermgr.PeerMgr
}

func NewHelloService(h host.Host, cs *store.ChainStore, syncer *chain.Syncer, cons consensus.Consensus, pmgr peermgr.MaybePeerMgr) *Service {
	if pmgr.Mgr == nil {
		log.Warn("running without peer manager")
	}

	return &Service{
		h: h,

		cs:     cs,
		syncer: syncer,
		cons:   cons,
		pmgr:   pmgr.Mgr,
	}
}

func (hs *Service) HandleStream(s inet.Stream) {
	var hmsg HelloMessage
	_ = s.SetReadDeadline(time.Now().Add(streamDeadline))
	if err := cborutil.ReadCborRPC(s, &hmsg); err != nil {
		_ = s.SetReadDeadline(time.Time{})
		log.Infow("failed to read hello message, disconnecting", "error", err)
		_ = s.Conn().Close()
		return
	}
	_ = s.SetReadDeadline(time.Time{})

	arrived := build.Clock.Now()

	log.Debugw("genesis from hello",
		"tipset", hmsg.HeaviestTipSet,
		"peer", s.Conn().RemotePeer(),
		"hash", hmsg.GenesisHash)

	if hmsg.GenesisHash != hs.syncer.Genesis.Cids()[0] {
		log.Debugf("other peer has different genesis! (%s)", hmsg.GenesisHash)
		_ = s.Conn().Close()
		return
	}
	go func() {
		defer s.Close() //nolint:errcheck

		sent := build.Clock.Now()
		msg := &LatencyMessage{
			TArrival: arrived.UnixNano(),
			TSent:    sent.UnixNano(),
		}
		_ = s.SetWriteDeadline(time.Now().Add(streamDeadline))
		if err := cborutil.WriteCborRPC(s, msg); err != nil {
			log.Debugf("error while responding to latency: %v", err)
		}
		_ = s.SetWriteDeadline(time.Time{})
	}()

	protos, err := hs.h.Peerstore().GetProtocols(s.Conn().RemotePeer())
	if err != nil {
		log.Warnf("got error from peerstore.GetProtocols: %s", err)
	}
	if len(protos) == 0 {
		log.Warn("other peer hasnt completed libp2p identify, waiting a bit")
		// TODO: this better
		build.Clock.Sleep(time.Millisecond * 300)
	}

	if hs.pmgr != nil {
		hs.pmgr.AddFilecoinPeer(s.Conn().RemotePeer())
	}

	// We're trying to fetch the tipset from the peer that just said hello to us. No point in
	// triggering any dials.
	ctx := network.WithNoDial(context.Background(), "fetching filecoin hello tipset")
	ts, err := hs.syncer.FetchTipSet(ctx, s.Conn().RemotePeer(), types.NewTipSetKey(hmsg.HeaviestTipSet...))
	if err != nil {
		log.Debugf("failed to fetch tipset from peer during hello: %+v", err)
		return
	}

	if ts.TipSet().Height() > 0 {
		hs.h.ConnManager().TagPeer(s.Conn().RemotePeer(), "fcpeer", 10)

		// don't bother informing about genesis
		log.Debugf("Got new tipset through Hello: %s from %s", ts.Cids(), s.Conn().RemotePeer())
		hs.syncer.InformNewHead(s.Conn().RemotePeer(), ts)
	}
}

func (hs *Service) SayHello(ctx context.Context, pid peer.ID) error {
	sctx, cancel := context.WithTimeout(ctx, streamOpenTimeout)
	defer cancel()
	s, err := hs.h.NewStream(sctx, pid, ProtocolID)
	if err != nil {
		return xerrors.Errorf("error opening stream: %w", err)
	}

	hts := hs.cs.GetHeaviestTipSet()
	weight, err := hs.cs.Weight(ctx, hts)
	if err != nil {
		return err
	}

	gen, err := hs.cs.GetGenesis(ctx)
	if err != nil {
		return err
	}

	hmsg := &HelloMessage{
		HeaviestTipSet:       hts.Cids(),
		HeaviestTipSetHeight: hts.Height(),
		HeaviestTipSetWeight: weight,
		GenesisHash:          gen.Cid(),
	}
	log.Debug("Sending hello message: ", hts.Cids(), hts.Height(), gen.Cid())

	t0 := build.Clock.Now()
	_ = s.SetWriteDeadline(time.Now().Add(streamDeadline))
	if err := cborutil.WriteCborRPC(s, hmsg); err != nil {
		_ = s.SetWriteDeadline(time.Time{})
		return xerrors.Errorf("writing rpc to peer: %w", err)
	}
	_ = s.SetWriteDeadline(time.Time{})
	if err := s.CloseWrite(); err != nil {
		log.Warnw("CloseWrite err", "error", err)
	}

	go func() {
		defer s.Close() //nolint:errcheck

		lmsg := &LatencyMessage{}
		_ = s.SetReadDeadline(build.Clock.Now().Add(10 * time.Second))
		err := cborutil.ReadCborRPC(s, lmsg)
		if err != nil {
			log.Debugw("reading latency message", "error", err)
		}

		t3 := build.Clock.Now()
		lat := t3.Sub(t0)
		// add to peer tracker
		if hs.pmgr != nil {
			hs.pmgr.SetPeerLatency(pid, lat)
		}

		if err == nil {
			if lmsg.TArrival != 0 && lmsg.TSent != 0 {
				t1 := time.Unix(0, lmsg.TArrival)
				t2 := time.Unix(0, lmsg.TSent)
				offset := t0.Sub(t1) + t3.Sub(t2)
				offset /= 2
				if offset > 5*time.Second || offset < -5*time.Second {
					log.Infow("time offset", "offset", offset.Seconds(), "peerid", pid.String())
				}
			}
		}
	}()

	return nil
}
