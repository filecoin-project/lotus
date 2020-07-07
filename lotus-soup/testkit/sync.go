package testkit

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/testground/sdk-go/sync"
)

var (
	GenesisTopic      = sync.NewTopic("genesis", &GenesisMsg{})
	BalanceTopic      = sync.NewTopic("balance", &InitialBalanceMsg{})
	PresealTopic      = sync.NewTopic("preseal", &PresealMsg{})
	ClientsAddrsTopic = sync.NewTopic("clients_addrs", &ClientAddressesMsg{})
	MinersAddrsTopic  = sync.NewTopic("miners_addrs", &MinerAddressesMsg{})
	PubsubTracerTopic = sync.NewTopic("pubsub_tracer", &PubsubTracerMsg{})
	DrandConfigTopic  = sync.NewTopic("drand_config", &DrandRuntimeInfo{})
)

var (
	StateReady           = sync.State("ready")
	StateDone            = sync.State("done")
	StateStopMining      = sync.State("stop-mining")
	StateMinerPickSeqNum = sync.State("miner-pick-seq-num")
)

type InitialBalanceMsg struct {
	Addr    address.Address
	Balance int
}

type PresealMsg struct {
	Miner genesis.Miner
	Seqno int64
}

type GenesisMsg struct {
	Genesis      []byte
	Bootstrapper []byte
}

type ClientAddressesMsg struct {
	PeerAddr   peer.AddrInfo
	WalletAddr address.Address
}

type MinerAddressesMsg struct {
	FullNetAddrs   peer.AddrInfo
	MinerNetAddrs  peer.AddrInfo
	MinerActorAddr address.Address
}

type PubsubTracerMsg struct {
	Multiaddr string
}

type DrandRuntimeInfo struct {
	Config          dtypes.DrandConfig
	GossipBootstrap dtypes.DrandBootstrap
}
