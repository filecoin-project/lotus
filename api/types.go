package api

import (
	"encoding/json"
	"fmt"
	"time"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
)

// TODO: check if this exists anywhere else

type MultiaddrSlice []ma.Multiaddr

func (m *MultiaddrSlice) UnmarshalJSON(raw []byte) (err error) {
	var temp []string
	if err := json.Unmarshal(raw, &temp); err != nil {
		return err
	}

	res := make([]ma.Multiaddr, len(temp))
	for i, str := range temp {
		res[i], err = ma.NewMultiaddr(str)
		if err != nil {
			return err
		}
	}
	*m = res
	return nil
}

var _ json.Unmarshaler = new(MultiaddrSlice)

type ObjStat struct {
	Size  uint64
	Links uint64
}

type PubsubScore struct {
	ID    peer.ID
	Score *pubsub.PeerScoreSnapshot
}

type MessageSendSpec struct {
	MaxFee abi.TokenAmount
}

type DataTransferChannel struct {
	TransferID  datatransfer.TransferID
	Status      datatransfer.Status
	BaseCID     cid.Cid
	IsInitiator bool
	IsSender    bool
	Voucher     string
	Message     string
	OtherPeer   peer.ID
	Transferred uint64
	Stages      *datatransfer.ChannelStages
}

// NewDataTransferChannel constructs an API DataTransferChannel type from full channel state snapshot and a host id
func NewDataTransferChannel(hostID peer.ID, channelState datatransfer.ChannelState) DataTransferChannel {
	channel := DataTransferChannel{
		TransferID: channelState.TransferID(),
		Status:     channelState.Status(),
		BaseCID:    channelState.BaseCID(),
		IsSender:   channelState.Sender() == hostID,
		Message:    channelState.Message(),
	}
	stringer, ok := channelState.Voucher().(fmt.Stringer)
	if ok {
		channel.Voucher = stringer.String()
	} else {
		voucherJSON, err := json.Marshal(channelState.Voucher())
		if err != nil {
			channel.Voucher = fmt.Errorf("Voucher Serialization: %w", err).Error()
		} else {
			channel.Voucher = string(voucherJSON)
		}
	}
	if channel.IsSender {
		channel.IsInitiator = !channelState.IsPull()
		channel.Transferred = channelState.Sent()
		channel.OtherPeer = channelState.Recipient()
	} else {
		channel.IsInitiator = channelState.IsPull()
		channel.Transferred = channelState.Received()
		channel.OtherPeer = channelState.Sender()
	}
	return channel
}

type NetBlockList struct {
	Peers     []peer.ID
	IPAddrs   []string
	IPSubnets []string
}

type ExtendedPeerInfo struct {
	ID          peer.ID
	Agent       string
	Addrs       []string
	Protocols   []string
	ConnMgrMeta *ConnMgrInfo
}

type ConnMgrInfo struct {
	FirstSeen time.Time
	Value     int
	Tags      map[string]int
	Conns     map[string]time.Time
}

type NodeStatus struct {
	SyncStatus  NodeSyncStatus
	PeerStatus  NodePeerStatus
	ChainStatus NodeChainStatus
}

type NodeSyncStatus struct {
	Epoch  uint64
	Behind uint64
}

type NodePeerStatus struct {
	PeersToPublishMsgs   int
	PeersToPublishBlocks int
}

type NodeChainStatus struct {
	BlocksPerTipsetLast100      float64
	BlocksPerTipsetLastFinality float64
}

type CheckStatusCode int

//go:generate go run golang.org/x/tools/cmd/stringer -type=CheckStatusCode -trimprefix=CheckStatus
const (
	_ CheckStatusCode = iota
	// Message Checks
	CheckStatusMessageSerialize
	CheckStatusMessageSize
	CheckStatusMessageValidity
	CheckStatusMessageMinGas
	CheckStatusMessageMinBaseFee
	CheckStatusMessageBaseFee
	CheckStatusMessageBaseFeeLowerBound
	CheckStatusMessageBaseFeeUpperBound
	CheckStatusMessageGetStateNonce
	CheckStatusMessageNonce
	CheckStatusMessageGetStateBalance
	CheckStatusMessageBalance
)

type CheckStatus struct {
	Code CheckStatusCode
	OK   bool
	Err  string
	Hint map[string]interface{}
}

type MessageCheckStatus struct {
	Cid cid.Cid
	CheckStatus
}

type MessagePrototype struct {
	Message    types.Message
	ValidNonce bool
}

type RetrievalInfo struct {
	PayloadCID   cid.Cid
	ID           retrievalmarket.DealID
	PieceCID     *cid.Cid
	PricePerByte abi.TokenAmount
	UnsealPrice  abi.TokenAmount

	Status        retrievalmarket.DealStatus
	Message       string // more information about deal state, particularly errors
	Provider      peer.ID
	BytesReceived uint64
	BytesPaidFor  uint64
	TotalPaid     abi.TokenAmount

	TransferChannelID *datatransfer.ChannelID
	DataTransfer      *DataTransferChannel

	// optional event if part of ClientGetRetrievalUpdates
	Event *retrievalmarket.ClientEvent
}

type RestrievalRes struct {
	DealID retrievalmarket.DealID
}

// Selector specifies ipld selector string
// - if the string starts with '{', it's interpreted as json selector string
//   see https://ipld.io/specs/selectors/ and https://ipld.io/specs/selectors/fixtures/selector-fixtures-1/
// - otherwise the string is interpreted as ipld-selector-text-lite (simple ipld path)
//   see https://github.com/ipld/go-ipld-selector-text-lite
type Selector string

type DagSpec struct {
	// DataSelector matches data to be retrieved
	// - when using textselector, the path specifies subtree
	// - the matched graph must have a single root
	DataSelector *Selector

	// ExportMerkleProof is applicable only when exporting to a CAR file via a path textselector
	// When true, in addition to the selection target, the resulting CAR will contain every block along the
	// path back to, and including the original root
	// When false the resulting CAR contains only the blocks of the target subdag
	ExportMerkleProof bool
}

type ExportRef struct {
	Root cid.Cid

	// DAGs array specifies a list of DAGs to export
	// - If exporting into unixfs files, only one DAG is supported, DataSelector is only used to find the targeted root node
	// - If exporting into a car file
	//   - When exactly one text-path DataSelector is specified exports the subgraph and its full merkle-path from the original root
	//   - Otherwise ( multiple paths and/or JSON selector specs) determines each individual subroot and exports the subtrees as a multi-root car
	// - When not specified defaults to a single DAG:
	//   - Data - the entire DAG: `{"R":{"l":{"none":{}},":>":{"a":{">":{"@":{}}}}}}`
	DAGs []DagSpec

	FromLocalCAR string // if specified, get data from a local CARv2 file.
	DealID       retrievalmarket.DealID
}
