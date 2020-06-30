package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/testground/sdk-go/sync"
)

var (
	genesisTopic      = sync.NewTopic("genesis", &GenesisMsg{})
	balanceTopic      = sync.NewTopic("balance", &InitialBalanceMsg{})
	presealTopic      = sync.NewTopic("preseal", &PresealMsg{})
	clientsAddrsTopic = sync.NewTopic("clientsAddrsTopic", &peer.AddrInfo{})
	minersAddrsTopic  = sync.NewTopic("minersAddrsTopic", &MinerAddressesMsg{})
	pubsubTracerTopic = sync.NewTopic("pubsubTracer", &PubsubTracerMsg{})
	drandConfigTopic  = sync.NewTopic("drand-config", &DrandRuntimeInfo{})
)

var (
	stateReady           = sync.State("ready")
	stateDone            = sync.State("done")
	stateStopMining      = sync.State("stop-mining")
	stateMinerPickSeqNum = sync.State("miner-pick-seq-num")
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

type MinerAddressesMsg struct {
	PeerAddr  peer.AddrInfo
	ActorAddr address.Address
}

type PubsubTracerMsg struct {
	Multiaddr string
}

type DrandRuntimeInfo struct {
	Config          dtypes.DrandConfig
	GossipBootstrap dtypes.DrandBootstrap
}
