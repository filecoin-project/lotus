package network

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-state-types/crypto"
)

// ResigningFunc allows you to resign data as needed when downgrading a response
type ResigningFunc func(ctx context.Context, data interface{}) (*crypto.Signature, error)

// These are the required interfaces that must be implemented to send and receive data
// for storage deals.

// StorageAskStream is a stream for reading/writing requests &
// responses on the Storage Ask protocol
type StorageAskStream interface {
	ReadAskRequest() (AskRequest, error)
	WriteAskRequest(AskRequest) error
	ReadAskResponse() (AskResponse, []byte, error)
	WriteAskResponse(AskResponse, ResigningFunc) error
	Close() error
}

// StorageDealStream is a stream for reading and writing requests
// and responses on the storage deal protocol
type StorageDealStream interface {
	ReadDealProposal() (Proposal, error)
	WriteDealProposal(Proposal) error
	ReadDealResponse() (SignedResponse, []byte, error)
	WriteDealResponse(SignedResponse, ResigningFunc) error
	RemotePeer() peer.ID
	Close() error
}

// DealStatusStream is a stream for reading and writing requests
// and responses on the deal status protocol
type DealStatusStream interface {
	ReadDealStatusRequest() (DealStatusRequest, error)
	WriteDealStatusRequest(DealStatusRequest) error
	ReadDealStatusResponse() (DealStatusResponse, []byte, error)
	WriteDealStatusResponse(DealStatusResponse, ResigningFunc) error
	Close() error
}

// StorageReceiver implements functions for receiving
// incoming data on storage protocols
type StorageReceiver interface {
	HandleAskStream(StorageAskStream)
	HandleDealStream(StorageDealStream)
	HandleDealStatusStream(DealStatusStream)
}

// StorageMarketNetwork is a network abstraction for the storage market
type StorageMarketNetwork interface {
	NewAskStream(context.Context, peer.ID) (StorageAskStream, error)
	NewDealStream(context.Context, peer.ID) (StorageDealStream, error)
	NewDealStatusStream(context.Context, peer.ID) (DealStatusStream, error)
	SetDelegate(StorageReceiver) error
	StopHandlingRequests() error
	ID() peer.ID
	AddAddrs(peer.ID, []ma.Multiaddr)

	PeerTagger
}

// PeerTagger implements arbitrary tagging of peers
type PeerTagger interface {
	TagPeer(peer.ID, string)
	UntagPeer(peer.ID, string)
}
