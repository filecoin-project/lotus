package api

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

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
	MaxFee  abi.TokenAmount
	MsgUuid uuid.UUID
}

type MpoolMessageWhole struct {
	Msg  *types.Message
	Spec *MessageSendSpec
}

// GraphSyncDataTransfer provides diagnostics on a data transfer happening over graphsync
type GraphSyncDataTransfer struct {
	// GraphSync request id for this transfer
	RequestID *graphsync.RequestID
	// Graphsync state for this transfer
	RequestState string
	// If a channel ID is present, indicates whether this is the current graphsync request for this channel
	// (could have changed in a restart)
	IsCurrentChannelRequest bool
	// Data transfer channel ID for this transfer
	ChannelID *datatransfer.ChannelID
	// Data transfer state for this transfer
	ChannelState *DataTransferChannel
	// Diagnostic information about this request -- and unexpected inconsistencies in
	// request state
	Diagnostics []string
}

// TransferDiagnostics give current information about transfers going over graphsync that may be helpful for debugging
type TransferDiagnostics struct {
	ReceivingTransfers []*GraphSyncDataTransfer
	SendingTransfers   []*GraphSyncDataTransfer
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

type NetStat struct {
	System    *network.ScopeStat           `json:",omitempty"`
	Transient *network.ScopeStat           `json:",omitempty"`
	Services  map[string]network.ScopeStat `json:",omitempty"`
	Protocols map[string]network.ScopeStat `json:",omitempty"`
	Peers     map[string]network.ScopeStat `json:",omitempty"`
}

type NetLimit struct {
	Memory int64 `json:",omitempty"`

	Streams, StreamsInbound, StreamsOutbound int
	Conns, ConnsInbound, ConnsOutbound       int
	FD                                       int
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
//   - if the string starts with '{', it's interpreted as json selector string
//     see https://ipld.io/specs/selectors/ and https://ipld.io/specs/selectors/fixtures/selector-fixtures-1/
//   - otherwise the string is interpreted as ipld-selector-text-lite (simple ipld path)
//     see https://github.com/ipld/go-ipld-selector-text-lite
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

type MinerInfo struct {
	Owner                      address.Address   // Must be an ID-address.
	Worker                     address.Address   // Must be an ID-address.
	NewWorker                  address.Address   // Must be an ID-address.
	ControlAddresses           []address.Address // Must be an ID-addresses.
	WorkerChangeEpoch          abi.ChainEpoch
	PeerId                     *peer.ID
	Multiaddrs                 []abi.Multiaddrs
	WindowPoStProofType        abi.RegisteredPoStProof
	SectorSize                 abi.SectorSize
	WindowPoStPartitionSectors uint64
	ConsensusFaultElapsed      abi.ChainEpoch
	Beneficiary                address.Address
	BeneficiaryTerm            *miner.BeneficiaryTerm
	PendingBeneficiaryTerm     *miner.PendingBeneficiaryChange
}

type NetworkParams struct {
	NetworkName             dtypes.NetworkName
	BlockDelaySecs          uint64
	ConsensusMinerMinPower  abi.StoragePower
	SupportedProofTypes     []abi.RegisteredSealProof
	PreCommitChallengeDelay abi.ChainEpoch
	ForkUpgradeParams       ForkUpgradeParams
}

type ForkUpgradeParams struct {
	UpgradeSmokeHeight         abi.ChainEpoch
	UpgradeBreezeHeight        abi.ChainEpoch
	UpgradeIgnitionHeight      abi.ChainEpoch
	UpgradeLiftoffHeight       abi.ChainEpoch
	UpgradeAssemblyHeight      abi.ChainEpoch
	UpgradeRefuelHeight        abi.ChainEpoch
	UpgradeTapeHeight          abi.ChainEpoch
	UpgradeKumquatHeight       abi.ChainEpoch
	UpgradePriceListOopsHeight abi.ChainEpoch
	BreezeGasTampingDuration   abi.ChainEpoch
	UpgradeCalicoHeight        abi.ChainEpoch
	UpgradePersianHeight       abi.ChainEpoch
	UpgradeOrangeHeight        abi.ChainEpoch
	UpgradeClausHeight         abi.ChainEpoch
	UpgradeTrustHeight         abi.ChainEpoch
	UpgradeNorwegianHeight     abi.ChainEpoch
	UpgradeTurboHeight         abi.ChainEpoch
	UpgradeHyperdriveHeight    abi.ChainEpoch
	UpgradeChocolateHeight     abi.ChainEpoch
	UpgradeOhSnapHeight        abi.ChainEpoch
	UpgradeSkyrHeight          abi.ChainEpoch
	UpgradeSharkHeight         abi.ChainEpoch
	UpgradeHyggeHeight         abi.ChainEpoch
}

type NonceMapType map[address.Address]uint64
type MsgUuidMapType map[uuid.UUID]*types.SignedMessage

type RaftStateData struct {
	NonceMap NonceMapType
	MsgUuids MsgUuidMapType
}

func (n *NonceMapType) MarshalJSON() ([]byte, error) {
	marshalled := make(map[string]uint64)
	for a, n := range *n {
		marshalled[a.String()] = n
	}
	return json.Marshal(marshalled)
}

func (n *NonceMapType) UnmarshalJSON(b []byte) error {
	unmarshalled := make(map[string]uint64)
	err := json.Unmarshal(b, &unmarshalled)
	if err != nil {
		return err
	}
	*n = make(map[address.Address]uint64)
	for saddr, nonce := range unmarshalled {
		a, err := address.NewFromString(saddr)
		if err != nil {
			return err
		}
		(*n)[a] = nonce
	}
	return nil
}

func (m *MsgUuidMapType) MarshalJSON() ([]byte, error) {
	marshalled := make(map[string]*types.SignedMessage)
	for u, msg := range *m {
		marshalled[u.String()] = msg
	}
	return json.Marshal(marshalled)
}

func (m *MsgUuidMapType) UnmarshalJSON(b []byte) error {
	unmarshalled := make(map[string]*types.SignedMessage)
	err := json.Unmarshal(b, &unmarshalled)
	if err != nil {
		return err
	}
	*m = make(map[uuid.UUID]*types.SignedMessage)
	for suid, msg := range unmarshalled {
		u, err := uuid.Parse(suid)
		if err != nil {
			return err
		}
		(*m)[u] = msg
	}
	return nil
}

type ChainExportConfig struct {
	WriteBufferSize   int
	Workers           int64
	CacheSize         int
	IncludeMessages   bool
	IncludeReceipts   bool
	IncludeStateRoots bool
}
