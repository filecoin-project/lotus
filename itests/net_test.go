//stm: #integration
package itests

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetConn(t *testing.T) {
	ctx := context.Background()

	firstNode, secondNode, _, _ := kit.EnsembleTwoOne(t)

	//stm: @NETWORK_COMMON_ID_001
	secondNodeID, err := secondNode.ID(ctx)
	require.NoError(t, err)

	connState := getConnState(ctx, t, firstNode, secondNodeID)

	if connState != network.NotConnected {
		t.Errorf("node should be not connected to peers. %s", err.Error())
	}

	//stm: @NETWORK_COMMON_ADDRS_LISTEN_001
	addrInfo, err := secondNode.NetAddrsListen(ctx)
	require.NoError(t, err)

	//stm: @NETWORK_COMMON_CONNECT_001
	err = firstNode.NetConnect(ctx, addrInfo)
	if err != nil {
		t.Errorf("nodes failed to connect. %s", err.Error())
	}

	//stm: @NETWORK_COMMON_PEER_INFO_001
	netPeerInfo, err := firstNode.NetPeerInfo(ctx, secondNodeID)
	require.NoError(t, err)

	//stm: @NETWORK_COMMON_AGENT_VERSION_001
	agent, err := firstNode.NetAgentVersion(ctx, secondNodeID)
	require.NoError(t, err)

	if netPeerInfo.Agent != agent {
		t.Errorf("agents not matching. %s", err.Error())
	}

	//stm: @NETWORK_COMMON_BANDWIDTH_STATS_001
	bandwidth, err := secondNode.NetBandwidthStats(ctx)
	require.NoError(t, err)

	//stm: @NETWORK_COMMON_BANDWIDTH_STATS_BY_PEER_001
	peerBandwidths, err := firstNode.NetBandwidthStatsByPeer(ctx)
	require.NoError(t, err)

	if peerBandwidths[secondNodeID.String()] != bandwidth {
		t.Errorf("bandwidths differ")
	}

	//stm: @NETWORK_COMMON_FIND_PEER_001
	secondNodePeer, err := firstNode.NetFindPeer(ctx, secondNodeID)
	require.NoError(t, err)

	if secondNodePeer.ID != addrInfo.ID {
		t.Errorf("peer id doesn't match with listen address.")
	}

	connState = getConnState(ctx, t, firstNode, secondNodeID)

	if connState != network.Connected {
		t.Errorf("peer does not have connected state")
	}

	//stm: @NETWORK_COMMON_PEERS_001
	peers, err := firstNode.NetPeers(ctx)
	require.NoError(t, err)

	if len(peers) > 0 && peers[0].ID != addrInfo.ID {
		t.Errorf("connected peer does not exist in network")
	}

	//stm: @NETWORK_COMMON_DISCONNECT_001
	err = firstNode.NetDisconnect(ctx, secondNodeID)
	if err != nil {
		t.Errorf("nodes failed to disconnect. %s", err.Error())
	}

	connState = getConnState(ctx, t, firstNode, secondNodeID)

	if connState != network.NotConnected {
		t.Errorf("peer should have disconnected")
	}

	//stm: @NETWORK_COMMON_PEERS_001
	peers, err = firstNode.NetPeers(ctx)
	require.NoError(t, err)

	if len(peers) > 0 {
		t.Errorf("there should be no peers in network after disconnecting node")
	}

}

func TestNetStat(t *testing.T) {

	firstNode, secondNode, _, _ := kit.EnsembleTwoOne(t)
	ctx := context.Background()

	sId, err := secondNode.ID(ctx)
	require.NoError(t, err)

	withScope := func(api interface{}, scope string) func(t *testing.T) {
		return func(t *testing.T) {

			stat, err := firstNode.NetStat(ctx, scope)
			require.NoError(t, err)

			switch scope {
			case "all":
				assert.NotNil(t, stat.System)
				assert.NotNil(t, stat.Transient)
			case "system":
				assert.NotNil(t, stat.System)
			case "transient":
				assert.NotNil(t, stat.Transient)
			}
		}
	}

	t.Run("all", withScope(t, "all"))
	t.Run("system", withScope(t, "system"))
	t.Run("transient", withScope(t, "transient"))
	t.Run("peer", withScope(t, fmt.Sprintf("peer:%s", sId)))
}

func TestNetLimit(t *testing.T) {

	firstNode, secondNode, _, _ := kit.EnsembleTwoOne(t)
	ctx := context.Background()

	sId, err := secondNode.ID(ctx)
	require.NoError(t, err)

	withScope := func(api interface{}, scope string) func(t *testing.T) {
		return func(t *testing.T) {
			_, err := firstNode.NetLimit(ctx, scope)
			require.NoError(t, err)
		}
	}

	t.Run("system", withScope(t, "system"))
	t.Run("transient", withScope(t, "transient"))
	t.Run("peer", withScope(t, fmt.Sprintf("peer:%s", sId)))
}

func TestNetBlockPeer(t *testing.T) {
	ctx := context.Background()

	firstNode, secondNode, _, ens := kit.EnsembleTwoOne(t)
	ens.InterconnectAll()

	//stm: @NETWORK_COMMON_ID_001
	secondNodeID, err := secondNode.ID(ctx)
	require.NoError(t, err)

	//stm: @NETWORK_COMMON_BLOCK_ADD_001
	err = firstNode.NetBlockAdd(ctx, api.NetBlockList{Peers: []peer.ID{secondNodeID}})
	require.NoError(t, err)

	//stm: @NETWORK_COMMON_BLOCK_LIST_001
	list, err := firstNode.NetBlockList(ctx)
	require.NoError(t, err)

	if len(list.Peers) == 0 || list.Peers[0] != secondNodeID {
		t.Errorf("blocked peer not in blocked peer list")
	}

	//stm: @NETWORK_COMMON_BLOCK_REMOVE_001
	err = firstNode.NetBlockRemove(ctx, api.NetBlockList{Peers: []peer.ID{secondNodeID}})
	require.NoError(t, err)

	//stm: @NETWORK_COMMON_BLOCK_LIST_001
	list, err = firstNode.NetBlockList(ctx)
	require.NoError(t, err)

	if len(list.Peers) > 0 {
		t.Errorf("failed to remove blocked peer from blocked peer list")
	}

}

func TestNetBlockIPAddr(t *testing.T) {
	ctx := context.Background()

	firstNode, secondNode, _, ens := kit.EnsembleTwoOne(t)
	ens.InterconnectAll()

	//stm: @NETWORK_COMMON_ADDRS_LISTEN_001
	addrInfo, _ := secondNode.NetAddrsListen(ctx)

	var secondNodeIPs []string

	for _, addr := range addrInfo.Addrs {
		ip, err := manet.ToIP(addr)
		if err != nil {
			continue
		}
		secondNodeIPs = append(secondNodeIPs, ip.String())
	}

	//stm: @NETWORK_COMMON_BLOCK_ADD_001
	err := firstNode.NetBlockAdd(ctx, api.NetBlockList{IPAddrs: secondNodeIPs})
	require.NoError(t, err)

	//stm: @NETWORK_COMMON_BLOCK_LIST_001
	list, err := firstNode.NetBlockList(ctx)
	require.NoError(t, err)

	if len(list.IPAddrs) == 0 || list.IPAddrs[0] != secondNodeIPs[0] {
		t.Errorf("blocked ip not in blocked ip list")
	}

	//stm: @NETWORK_COMMON_BLOCK_REMOVE_001
	err = firstNode.NetBlockRemove(ctx, api.NetBlockList{IPAddrs: secondNodeIPs})
	require.NoError(t, err)

	//stm: @NETWORK_COMMON_BLOCK_LIST_001
	list, err = firstNode.NetBlockList(ctx)
	require.NoError(t, err)

	if len(list.IPAddrs) > 0 {
		t.Errorf("failed to remove blocked ip from blocked ip list")
	}

}

func getConnState(ctx context.Context, t *testing.T, node *kit.TestFullNode, peer peer.ID) network.Connectedness {
	//stm: @NETWORK_COMMON_CONNECTEDNESS_001
	connState, err := node.NetConnectedness(ctx, peer)
	require.NoError(t, err)

	return connState
}
