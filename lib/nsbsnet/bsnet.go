package nsbsnet

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/protocol"
	"io"
	"sync/atomic"
	"time"

	bsmsg "github.com/ipfs/go-bitswap/message"
	bsnet "github.com/ipfs/go-bitswap/network"
	"github.com/libp2p/go-libp2p-core/helpers"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	peerstore "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/routing"
	msgio "github.com/libp2p/go-msgio"
	ma "github.com/multiformats/go-multiaddr"
)

// TODO: Upstream to bitswap

var log = logging.Logger("nsbsnet")

var sendMessageTimeout = time.Minute * 10

// NewFromIpfsHost returns a BitSwapNetwork supported by underlying IPFS host.
func NewFromIpfsHost(host host.Host, r routing.ContentRouting, prefix protocol.ID) bsnet.BitSwapNetwork {
	bitswapNetwork := impl{
		host:    host,
		routing: r,
		prefix:  prefix,
	}
	return &bitswapNetwork
}

// impl transforms the ipfs network interface, which sends and receives
// NetMessage objects, into the bitswap network interface.
type impl struct {
	host    host.Host
	routing routing.ContentRouting
	prefix  protocol.ID

	// inbound messages from the network are forwarded to the receiver
	receiver bsnet.Receiver

	stats bsnet.Stats
}

type streamMessageSender struct {
	i *impl
	s network.Stream
}

func (i *impl) ProtocolBitswap() protocol.ID {
	return i.prefix + bsnet.ProtocolBitswap
}
func (i *impl) ProtocolBitswapOne() protocol.ID {
	return i.prefix + bsnet.ProtocolBitswapOne
}
func (i *impl) ProtocolBitswapNoVers() protocol.ID {
	return i.prefix + bsnet.ProtocolBitswapNoVers
}
func (s *streamMessageSender) Close() error {
	return helpers.FullClose(s.s)
}

func (s *streamMessageSender) Reset() error {
	return s.s.Reset()
}

func (s *streamMessageSender) SendMsg(ctx context.Context, msg bsmsg.BitSwapMessage) error {
	return s.i.msgToStream(ctx, s.s, msg)
}

func (i *impl) msgToStream(ctx context.Context, s network.Stream, msg bsmsg.BitSwapMessage) error {
	deadline := time.Now().Add(sendMessageTimeout)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}

	if err := s.SetWriteDeadline(deadline); err != nil {
		log.Warningf("error setting deadline: %s", err)
	}

	switch s.Protocol() {
	case i.ProtocolBitswap():
		if err := msg.ToNetV1(s); err != nil {
			log.Debugf("error: %s", err)
			return err
		}
	case i.ProtocolBitswapOne(), i.ProtocolBitswapNoVers():
		if err := msg.ToNetV0(s); err != nil {
			log.Debugf("error: %s", err)
			return err
		}
	default:
		return fmt.Errorf("unrecognized protocol on remote: %s", s.Protocol())
	}

	if err := s.SetWriteDeadline(time.Time{}); err != nil {
		log.Warningf("error resetting deadline: %s", err)
	}
	return nil
}

func (i *impl) NewMessageSender(ctx context.Context, p peer.ID) (bsnet.MessageSender, error) {
	s, err := i.newStreamToPeer(ctx, p)
	if err != nil {
		return nil, err
	}

	return &streamMessageSender{i: i, s: s}, nil
}

func (i *impl) newStreamToPeer(ctx context.Context, p peer.ID) (network.Stream, error) {
	return i.host.NewStream(ctx, p, i.ProtocolBitswap(), i.ProtocolBitswapOne(), i.ProtocolBitswapNoVers())
}

func (i *impl) SendMessage(
	ctx context.Context,
	p peer.ID,
	outgoing bsmsg.BitSwapMessage) error {

	s, err := i.newStreamToPeer(ctx, p)
	if err != nil {
		return err
	}

	if err = i.msgToStream(ctx, s, outgoing); err != nil {
		s.Reset()
		return err
	}
	atomic.AddUint64(&i.stats.MessagesSent, 1)

	// TODO(https://github.com/libp2p/go-libp2p-net/issues/28): Avoid this goroutine.
	go helpers.AwaitEOF(s)
	return s.Close()

}

func (i *impl) SetDelegate(r bsnet.Receiver) {
	i.receiver = r
	i.host.SetStreamHandler(i.ProtocolBitswap(), i.handleNewStream)
	i.host.SetStreamHandler(i.ProtocolBitswapOne(), i.handleNewStream)
	i.host.SetStreamHandler(i.ProtocolBitswapNoVers(), i.handleNewStream)
	i.host.Network().Notify((*netNotifiee)(i))
	// TODO: StopNotify.

}

func (i *impl) ConnectTo(ctx context.Context, p peer.ID) error {
	return i.host.Connect(ctx, peer.AddrInfo{ID: p})
}

// FindProvidersAsync returns a channel of providers for the given key.
func (i *impl) FindProvidersAsync(ctx context.Context, k cid.Cid, max int) <-chan peer.ID {
	out := make(chan peer.ID, max)
	go func() {
		defer close(out)
		providers := i.routing.FindProvidersAsync(ctx, k, max)
		for info := range providers {
			if info.ID == i.host.ID() {
				continue // ignore self as provider
			}
			i.host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.TempAddrTTL)
			select {
			case <-ctx.Done():
				return
			case out <- info.ID:
			}
		}
	}()
	return out
}

// Provide provides the key to the network
func (i *impl) Provide(ctx context.Context, k cid.Cid) error {
	return i.routing.Provide(ctx, k, true)
}

// handleNewStream receives a new stream from the network.
func (i *impl) handleNewStream(s network.Stream) {
	defer s.Close()

	if i.receiver == nil {
		s.Reset()
		return
	}

	reader := msgio.NewVarintReaderSize(s, network.MessageSizeMax)
	for {
		received, err := bsmsg.FromMsgReader(reader)
		if err != nil {
			if err != io.EOF {
				s.Reset()
				go i.receiver.ReceiveError(err)
				log.Debugf("bitswap net handleNewStream from %s error: %s", s.Conn().RemotePeer(), err)
			}
			return
		}

		p := s.Conn().RemotePeer()
		ctx := context.Background()
		log.Debugf("bitswap net handleNewStream from %s", s.Conn().RemotePeer())
		i.receiver.ReceiveMessage(ctx, p, received)
		atomic.AddUint64(&i.stats.MessagesRecvd, 1)
	}
}

func (i *impl) ConnectionManager() connmgr.ConnManager {
	return i.host.ConnManager()
}

func (i *impl) Stats() bsnet.Stats {
	return bsnet.Stats{
		MessagesRecvd: atomic.LoadUint64(&i.stats.MessagesRecvd),
		MessagesSent:  atomic.LoadUint64(&i.stats.MessagesSent),
	}
}

type netNotifiee impl

func (nn *netNotifiee) impl() *impl {
	return (*impl)(nn)
}

func (nn *netNotifiee) Connected(n network.Network, v network.Conn) {
	nn.impl().receiver.PeerConnected(v.RemotePeer())
}

func (nn *netNotifiee) Disconnected(n network.Network, v network.Conn) {
	nn.impl().receiver.PeerDisconnected(v.RemotePeer())
}

func (nn *netNotifiee) OpenedStream(n network.Network, v network.Stream) {}
func (nn *netNotifiee) ClosedStream(n network.Network, v network.Stream) {}
func (nn *netNotifiee) Listen(n network.Network, a ma.Multiaddr)         {}
func (nn *netNotifiee) ListenClose(n network.Network, a ma.Multiaddr)    {}
