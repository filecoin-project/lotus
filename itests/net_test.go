package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestNetConn(t *testing.T) {
	ctx := context.Background()

	firstNode, secondNode, _, _ := kit.EnsembleTwoOne(t)

	secondNodeID, err := secondNode.ID(ctx)
	require.NoError(t, err)

	connState := getConnState(ctx, t, firstNode, secondNodeID)

	if connState != network.NotConnected {
		t.Errorf("node should be not connected to peers. %s", err.Error())
	}

	addrInfo, err := secondNode.NetAddrsListen(ctx)
	require.NoError(t, err)

	err = firstNode.NetConnect(ctx, addrInfo)
	if err != nil {
		t.Errorf("nodes failed to connect. %s", err.Error())
	}

	netPeerInfo, err := firstNode.NetPeerInfo(ctx, secondNodeID)
	require.NoError(t, err)

	agent, err := firstNode.NetAgentVersion(ctx, secondNodeID)
	require.NoError(t, err)

	if netPeerInfo.Agent != agent {
		t.Errorf("agents not matching. %s", err.Error())
	}

	secondNodePeer, err := firstNode.NetFindPeer(ctx, secondNodeID)
	require.NoError(t, err)

	if secondNodePeer.ID != addrInfo.ID {
		t.Errorf("peer id doesn't match with listen address.")
	}

	connState = getConnState(ctx, t, firstNode, secondNodeID)

	if connState != network.Connected {
		t.Errorf("peer does not have connected state")
	}

	addrs, err := firstNode.NetPeers(ctx)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(addrs))

	err = firstNode.NetDisconnect(ctx, secondNodeID)
	if err != nil {
		t.Errorf("nodes failed to disconnect. %s", err.Error())
	}

	connState = getConnState(ctx, t, firstNode, secondNodeID)

	if connState != network.NotConnected {
		t.Errorf("peer should have disconnected")
	}

	addrs, err = firstNode.NetPeers(ctx)
	require.NoError(t, err)

	if len(addrs) > 0 {
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

	firstNode, secondNode, _, _ := kit.EnsembleTwoOne(t)

	firstAddrInfo, _ := firstNode.NetAddrsListen(ctx)
	firstNodeID, err := firstNode.ID(ctx)
	require.NoError(t, err)
	secondNodeID, err := secondNode.ID(ctx)
	require.NoError(t, err)

	// Sanity check that we're not already connected somehow
	connectedness, err := secondNode.NetConnectedness(ctx, firstNodeID)
	require.NoError(t, err, "failed to determine connectedness")
	require.NotEqual(t, connectedness, network.Connected, "shouldn't already be connected")

	err = firstNode.NetBlockAdd(ctx, api.NetBlockList{Peers: []peer.ID{secondNodeID}})
	require.NoError(t, err)

	list, err := firstNode.NetBlockList(ctx)
	require.NoError(t, err)

	if len(list.Peers) == 0 || list.Peers[0] != secondNodeID {
		t.Errorf("blocked peer not in blocked peer list")
	}

	err = secondNode.NetConnect(ctx, firstAddrInfo)
	// With early muxer selection, we'll only learn that the handshake failed
	// when we do something with the connection, for example when we open a stream.
	if err == nil {
		_, err = secondNode.NetPing(context.Background(), firstAddrInfo.ID)
	}
	require.Error(t, err, "shouldn't be able to connect to second node")
	connectedness, err = firstNode.NetConnectedness(ctx, secondNodeID)
	require.NoError(t, err, "failed to determine connectedness")
	require.NotEqual(t, connectedness, network.Connected)

	err = firstNode.NetBlockRemove(ctx, api.NetBlockList{Peers: []peer.ID{secondNodeID}})
	require.NoError(t, err)

	list, err = firstNode.NetBlockList(ctx)
	require.NoError(t, err)

	if len(list.Peers) > 0 {
		t.Errorf("failed to remove blocked peer from blocked peer list")
	}

	require.NoError(t, secondNode.NetConnect(ctx, firstAddrInfo), "failed to connect to second node")
	connectedness, err = secondNode.NetConnectedness(ctx, firstAddrInfo.ID)
	require.NoError(t, err, "failed to determine connectedness")
	require.Equal(t, connectedness, network.Connected)
}

func TestNetBlockIPAddr(t *testing.T) {
	ctx := context.Background()

	firstNode, secondNode, _, _ := kit.EnsembleTwoOne(t)

	firstAddrInfo, _ := firstNode.NetAddrsListen(ctx)
	secondAddrInfo, _ := secondNode.NetAddrsListen(ctx)

	secondNodeIPsMap := map[string]struct{}{} // so we can deduplicate
	for _, addr := range secondAddrInfo.Addrs {
		ip, err := manet.ToIP(addr)
		if err != nil {
			continue
		}
		secondNodeIPsMap[ip.String()] = struct{}{}
	}
	var secondNodeIPs []string
	for s := range secondNodeIPsMap {
		secondNodeIPs = append(secondNodeIPs, s)
	}

	// Sanity check that we're not already connected somehow
	connectedness, err := secondNode.NetConnectedness(ctx, firstAddrInfo.ID)
	require.NoError(t, err, "failed to determine connectedness")
	require.NotEqual(t, connectedness, network.Connected, "shouldn't already be connected")

	require.NoError(t, firstNode.NetBlockAdd(ctx, api.NetBlockList{
		IPAddrs: secondNodeIPs}), "failed to add blocked IPs")

	list, err := firstNode.NetBlockList(ctx)
	require.NoError(t, err)

	fmt.Println(list.IPAddrs)
	fmt.Println(secondNodeIPs)
	require.Equal(t, len(list.IPAddrs), len(secondNodeIPs), "expected %d blocked IPs", len(secondNodeIPs))
	for _, blockedIP := range list.IPAddrs {
		found := false
		for _, secondNodeIP := range secondNodeIPs {
			if blockedIP == secondNodeIP {
				found = true
				break
			}
		}

		require.True(t, found, "blocked IP %s is not one of secondNodeIPs", blockedIP)
	}

	// a QUIC connection might still succeed when gated, but will be killed right after the handshake
	_ = secondNode.NetConnect(ctx, firstAddrInfo)

	require.Eventually(t, func() bool {
		connectedness, err = secondNode.NetConnectedness(ctx, firstAddrInfo.ID)
		require.NoError(t, err, "failed to determine connectedness")
		return connectedness != network.Connected
	}, time.Second*5, time.Millisecond*10)

	err = firstNode.NetBlockRemove(ctx, api.NetBlockList{IPAddrs: secondNodeIPs})
	require.NoError(t, err)

	list, err = firstNode.NetBlockList(ctx)
	require.NoError(t, err)

	if len(list.IPAddrs) > 0 {
		t.Errorf("failed to remove blocked ip from blocked ip list")
	}

	require.NoError(t, secondNode.NetConnect(ctx, firstAddrInfo), "failed to connect to second node")
	connectedness, err = secondNode.NetConnectedness(ctx, firstAddrInfo.ID)
	require.NoError(t, err, "failed to determine connectedness")
	require.Equal(t, connectedness, network.Connected)
}

func getConnState(ctx context.Context, t *testing.T, node *kit.TestFullNode, peer peer.ID) network.Connectedness {
	connState, err := node.NetConnectedness(ctx, peer)
	require.NoError(t, err)

	return connState
}
