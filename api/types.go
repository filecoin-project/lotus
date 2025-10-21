package api

import (
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type ObjStat struct {
	Size  uint64
	Links uint64
}

type PubsubScore struct {
	ID    peer.ID
	Score *pubsub.PeerScoreSnapshot
}

// MessageSendSpec contains optional fields which modify message sending behavior
type MessageSendSpec struct {
	// MaxFee specifies a cap on network fees related to this message
	MaxFee abi.TokenAmount

	// MsgUuid specifies a unique message identifier which can be used on node (or node cluster)
	// level to prevent double-sends of messages even when nonce generation is not handled by sender
	MsgUuid uuid.UUID

	// MaximizeFeeCap makes message FeeCap be based entirely on MaxFee
	MaximizeFeeCap bool
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
	PendingOwnerAddress        *address.Address
	Beneficiary                address.Address
	BeneficiaryTerm            *miner.BeneficiaryTerm
	PendingBeneficiaryTerm     *miner.PendingBeneficiaryChange
}

type NetworkParams struct {
	NetworkName             dtypes.NetworkName
	BlockDelaySecs          uint64
	ConsensusMinerMinPower  abi.StoragePower
	PreCommitChallengeDelay abi.ChainEpoch
	ForkUpgradeParams       ForkUpgradeParams
	Eip155ChainID           int
	GenesisTimestamp        uint64
}

type ForkUpgradeParams struct {
	UpgradeSmokeHeight       abi.ChainEpoch
	UpgradeBreezeHeight      abi.ChainEpoch
	UpgradeIgnitionHeight    abi.ChainEpoch
	UpgradeLiftoffHeight     abi.ChainEpoch
	UpgradeAssemblyHeight    abi.ChainEpoch
	UpgradeRefuelHeight      abi.ChainEpoch
	UpgradeTapeHeight        abi.ChainEpoch
	UpgradeKumquatHeight     abi.ChainEpoch
	BreezeGasTampingDuration abi.ChainEpoch
	UpgradeCalicoHeight      abi.ChainEpoch
	UpgradePersianHeight     abi.ChainEpoch
	UpgradeOrangeHeight      abi.ChainEpoch
	UpgradeClausHeight       abi.ChainEpoch
	UpgradeTrustHeight       abi.ChainEpoch
	UpgradeNorwegianHeight   abi.ChainEpoch
	UpgradeTurboHeight       abi.ChainEpoch
	UpgradeHyperdriveHeight  abi.ChainEpoch
	UpgradeChocolateHeight   abi.ChainEpoch
	UpgradeOhSnapHeight      abi.ChainEpoch
	UpgradeSkyrHeight        abi.ChainEpoch
	UpgradeSharkHeight       abi.ChainEpoch
	UpgradeHyggeHeight       abi.ChainEpoch
	UpgradeLightningHeight   abi.ChainEpoch
	UpgradeThunderHeight     abi.ChainEpoch
	UpgradeWatermelonHeight  abi.ChainEpoch
	UpgradeDragonHeight      abi.ChainEpoch
	UpgradePhoenixHeight     abi.ChainEpoch
	UpgradeWaffleHeight      abi.ChainEpoch
	UpgradeTuktukHeight      abi.ChainEpoch
	UpgradeTeepHeight        abi.ChainEpoch
	UpgradeTockHeight        abi.ChainEpoch
	UpgradeGoldenWeekHeight  abi.ChainEpoch
	UpgradeXxHeight          abi.ChainEpoch
}

// ChainExportConfig holds configuration for chain ranged exports.
type ChainExportConfig struct {
	WriteBufferSize   int
	NumWorkers        int
	IncludeMessages   bool
	IncludeReceipts   bool
	IncludeStateRoots bool
}
