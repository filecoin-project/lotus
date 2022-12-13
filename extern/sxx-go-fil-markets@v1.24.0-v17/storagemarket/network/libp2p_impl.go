package network

import (
	"bufio"
	"context"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

var log = logging.Logger("storagemarket_network")

// Option is an option for configuring the libp2p storage market network
type Option func(*libp2pStorageMarketNetwork)

// RetryParameters changes the default parameters around connection reopening
func RetryParameters(minDuration time.Duration, maxDuration time.Duration, attempts float64, backoffFactor float64) Option {
	return func(impl *libp2pStorageMarketNetwork) {
		impl.retryStream.SetOptions(shared.RetryParameters(minDuration, maxDuration, attempts, backoffFactor))
	}
}

// SupportedAskProtocols sets what ask protocols this network instances listens on
func SupportedAskProtocols(supportedProtocols []protocol.ID) Option {
	return func(impl *libp2pStorageMarketNetwork) {
		impl.supportedAskProtocols = supportedProtocols
	}
}

// SupportedDealProtocols sets what deal protocols this network instances listens on
func SupportedDealProtocols(supportedProtocols []protocol.ID) Option {
	return func(impl *libp2pStorageMarketNetwork) {
		impl.supportedDealProtocols = supportedProtocols
	}
}

// SupportedDealStatusProtocols sets what deal status protocols this network instances listens on
func SupportedDealStatusProtocols(supportedProtocols []protocol.ID) Option {
	return func(impl *libp2pStorageMarketNetwork) {
		impl.supportedDealStatusProtocols = supportedProtocols
	}
}

// NewFromLibp2pHost builds a storage market network on top of libp2p
func NewFromLibp2pHost(h host.Host, options ...Option) StorageMarketNetwork {
	impl := &libp2pStorageMarketNetwork{
		host:        h,
		retryStream: shared.NewRetryStream(h),
		supportedAskProtocols: []protocol.ID{
			storagemarket.AskProtocolID,
			storagemarket.OldAskProtocolID,
		},
		supportedDealProtocols: []protocol.ID{
			storagemarket.DealProtocolID111,
			storagemarket.DealProtocolID110,
			storagemarket.DealProtocolID101,
		},
		supportedDealStatusProtocols: []protocol.ID{
			storagemarket.DealStatusProtocolID,
			storagemarket.OldDealStatusProtocolID,
		},
	}
	for _, option := range options {
		option(impl)
	}
	return impl
}

// libp2pStorageMarketNetwork transforms the libp2p host interface, which sends and receives
// NetMessage objects, into the graphsync network interface.
type libp2pStorageMarketNetwork struct {
	host        host.Host
	retryStream *shared.RetryStream
	// inbound messages from the network are forwarded to the receiver
	receiver                     StorageReceiver
	supportedAskProtocols        []protocol.ID
	supportedDealProtocols       []protocol.ID
	supportedDealStatusProtocols []protocol.ID
}

func (impl *libp2pStorageMarketNetwork) NewAskStream(ctx context.Context, id peer.ID) (StorageAskStream, error) {
	s, err := impl.retryStream.OpenStream(ctx, id, impl.supportedAskProtocols)
	if err != nil {
		log.Warn(err)
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	if s.Protocol() == storagemarket.OldAskProtocolID {
		return &legacyAskStream{p: id, rw: s, buffered: buffered}, nil
	}
	return &askStream{p: id, rw: s, buffered: buffered}, nil
}

func (impl *libp2pStorageMarketNetwork) NewDealStream(ctx context.Context, id peer.ID) (StorageDealStream, error) {
	s, err := impl.retryStream.OpenStream(ctx, id, impl.supportedDealProtocols)
	if err != nil {
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	switch s.Protocol() {
	case storagemarket.DealProtocolID101:
		return &dealStreamv101{p: id, rw: s, buffered: buffered, host: impl.host}, nil
	case storagemarket.DealProtocolID110:
		return &dealStreamv110{p: id, rw: s, buffered: buffered, host: impl.host}, nil
	default:
		return &dealStreamv111{p: id, rw: s, buffered: buffered, host: impl.host}, nil
	}
}

func (impl *libp2pStorageMarketNetwork) NewDealStatusStream(ctx context.Context, id peer.ID) (DealStatusStream, error) {
	s, err := impl.retryStream.OpenStream(ctx, id, impl.supportedDealStatusProtocols)
	if err != nil {
		log.Warn(err)
		return nil, err
	}
	buffered := bufio.NewReaderSize(s, 16)
	if s.Protocol() == storagemarket.OldDealStatusProtocolID {
		return &legacyDealStatusStream{p: id, rw: s, buffered: buffered}, nil
	}
	return &dealStatusStream{p: id, rw: s, buffered: buffered}, nil
}

func (impl *libp2pStorageMarketNetwork) SetDelegate(r StorageReceiver) error {
	impl.receiver = r
	for _, proto := range impl.supportedAskProtocols {
		impl.host.SetStreamHandler(proto, impl.handleNewAskStream)
	}
	for _, proto := range impl.supportedDealProtocols {
		impl.host.SetStreamHandler(proto, impl.handleNewDealStream)
	}
	for _, proto := range impl.supportedDealStatusProtocols {
		impl.host.SetStreamHandler(proto, impl.handleNewDealStatusStream)
	}
	return nil
}

func (impl *libp2pStorageMarketNetwork) StopHandlingRequests() error {
	impl.receiver = nil
	for _, proto := range impl.supportedAskProtocols {
		impl.host.RemoveStreamHandler(proto)
	}
	for _, proto := range impl.supportedDealProtocols {
		impl.host.RemoveStreamHandler(proto)
	}
	for _, proto := range impl.supportedDealStatusProtocols {
		impl.host.RemoveStreamHandler(proto)
	}
	return nil
}

func (impl *libp2pStorageMarketNetwork) handleNewAskStream(s network.Stream) {
	reader := impl.getReaderOrReset(s)
	if reader != nil {
		var as StorageAskStream
		if s.Protocol() == storagemarket.OldAskProtocolID {
			as = &legacyAskStream{s.Conn().RemotePeer(), s, reader}
		} else {
			as = &askStream{s.Conn().RemotePeer(), s, reader}
		}
		impl.receiver.HandleAskStream(as)
	}
}

func (impl *libp2pStorageMarketNetwork) handleNewDealStream(s network.Stream) {
	reader := impl.getReaderOrReset(s)
	if reader != nil {
		var ds StorageDealStream
		switch s.Protocol() {
		case storagemarket.DealProtocolID101:
			ds = &dealStreamv101{s.Conn().RemotePeer(), impl.host, s, reader}
		case storagemarket.DealProtocolID110:
			ds = &dealStreamv110{s.Conn().RemotePeer(), impl.host, s, reader}
		default:
			ds = &dealStreamv111{s.Conn().RemotePeer(), impl.host, s, reader}
		}
		impl.receiver.HandleDealStream(ds)
	}
}

func (impl *libp2pStorageMarketNetwork) handleNewDealStatusStream(s network.Stream) {
	reader := impl.getReaderOrReset(s)
	if reader != nil {
		var qs DealStatusStream
		if s.Protocol() == storagemarket.OldDealStatusProtocolID {
			qs = &legacyDealStatusStream{s.Conn().RemotePeer(), impl.host, s, reader}
		} else {
			qs = &dealStatusStream{s.Conn().RemotePeer(), impl.host, s, reader}
		}
		impl.receiver.HandleDealStatusStream(qs)
	}
}

func (impl *libp2pStorageMarketNetwork) getReaderOrReset(s network.Stream) *bufio.Reader {
	if impl.receiver == nil {
		log.Warn("no receiver set")
		s.Reset() // nolint: errcheck,gosec
		return nil
	}
	return bufio.NewReaderSize(s, 16)
}

func (impl *libp2pStorageMarketNetwork) ID() peer.ID {
	return impl.host.ID()
}

func (impl *libp2pStorageMarketNetwork) AddAddrs(p peer.ID, addrs []ma.Multiaddr) {
	impl.host.Peerstore().AddAddrs(p, addrs, 8*time.Hour)
}

func (impl *libp2pStorageMarketNetwork) TagPeer(p peer.ID, id string) {
	impl.host.ConnManager().TagPeer(p, id, TagPriority)
}

func (impl *libp2pStorageMarketNetwork) UntagPeer(p peer.ID, id string) {
	impl.host.ConnManager().UntagPeer(p, id)
}
