//stm: #cli
package cli

import (
	"context"
	"testing"

	"github.com/filecoin-project/lotus/api"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	"github.com/stretchr/testify/assert"
)

func TestNetPeers(t *testing.T) {
	peerAddrInfo := []peer.AddrInfo{}
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetPeers))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockApi.EXPECT().NetPeers(ctx).Return(peerAddrInfo, nil)

	//stm: @CLI_NETWORK_PEERS_001
	err := app.Run([]string{"net", "peers"})
	assert.NoError(t, err)
}

func TestNetScores(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetScores))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pubsubScores := []api.PubsubScore{}

	mockApi.EXPECT().NetPubsubScores(ctx).Return(pubsubScores, nil)

	//stm: @CLI_NETWORK_SCORES_001
	err := app.Run([]string{"net", "scores"})
	assert.NoError(t, err)
}

func TestNetListen(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetListen))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrInfo := peer.AddrInfo{}

	mockApi.EXPECT().NetAddrsListen(ctx).Return(addrInfo, nil)

	//stm: @CLI_NETWORK_LISTEN_001
	err := app.Run([]string{"net", "listen"})
	assert.NoError(t, err)
}

func TestNetReachability(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetReachability))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	natInfo := api.NatInfo{}

	mockApi.EXPECT().NetAutoNatStatus(ctx).Return(natInfo, nil)

	//stm: @CLI_NETWORK_REACHABILITY_001
	err := app.Run([]string{"net", "reachability"})
	assert.NoError(t, err)
}

func TestNetId(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetId))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var id peer.ID

	mockApi.EXPECT().ID(ctx).Return(id, nil)

	//stm: @CLI_NETWORK_ID_001
	err := app.Run([]string{"net", "id"})
	assert.NoError(t, err)
}

func TestNetBandwidthCmd(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBandwidthCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stats := metrics.Stats{}

		mockApi.EXPECT().NetBandwidthStats(ctx).Return(stats, nil)

		//stm: @CLI_NETWORK_BANDWIDTH_001
		err := app.Run([]string{"net", "bandwidth"})
		assert.NoError(t, err)
	})
	t.Run("by-peer", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBandwidthCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stats := make(map[string]metrics.Stats)

		mockApi.EXPECT().NetBandwidthStatsByPeer(ctx).Return(stats, nil)

		//stm: @CLI_NETWORK_BANDWIDTH_002
		err := app.Run([]string{"net", "bandwidth", "--by-peer"})
		assert.NoError(t, err)
	})
	t.Run("by-protocol", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBandwidthCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stats := make(map[protocol.ID]metrics.Stats)

		mockApi.EXPECT().NetBandwidthStatsByProtocol(ctx).Return(stats, nil)

		//stm: @CLI_NETWORK_BANDWIDTH_003
		err := app.Run([]string{"net", "bandwidth", "--by-protocol"})
		assert.NoError(t, err)
	})
}

func TestNetFindPeer(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetFindPeer))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrInfo := peer.AddrInfo{}
	pid := "12D3KooWLSrbbvvvujrHrvwao6hRn1Gz8vn9eAR8QsDDKZz4xES6"
	decoded, err := peer.Decode(pid)
	assert.NoError(t, err)

	mockApi.EXPECT().NetFindPeer(ctx, peer.ID(decoded)).Return(addrInfo, nil)

	//stm: @CLI_NETWORK_FIND_PEER_001
	err = app.Run([]string{"net", "findpeer", pid})
	assert.NoError(t, err)
}

func TestNetBlockPeer(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBlockCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pid := "12D3KooWLSrbbvvvujrHrvwao6hRn1Gz8vn9eAR8QsDDKZz4xES6"
	decoded, err := peer.Decode(pid)
	assert.NoError(t, err)

	mockApi.EXPECT().NetBlockAdd(ctx, api.NetBlockList{Peers: []peer.ID{peer.ID(decoded)}})

	//stm: @CLI_NETWORK_BLOCK_PEER_001
	err = app.Run([]string{"net", "block", "add", "peer", pid})
	assert.NoError(t, err)
}

func TestNetBlockIP(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBlockCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ip := "192.168.0.1/24"

	mockApi.EXPECT().NetBlockAdd(ctx, api.NetBlockList{IPAddrs: []string{ip}})

	//stm: @CLI_NETWORK_BLOCK_IP_001
	err := app.Run([]string{"net", "block", "add", "ip", ip})
	assert.NoError(t, err)
}

func TestNetBlockSubnet(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBlockCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subnet := "255.255.255.0/24"

	mockApi.EXPECT().NetBlockAdd(ctx, api.NetBlockList{IPSubnets: []string{subnet}})

	//stm: @CLI_NETWORK_BLOCK_SUBNET_001
	err := app.Run([]string{"net", "block", "add", "subnet", subnet})
	assert.NoError(t, err)
}

func TestNetBlockList(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBlockCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockApi.EXPECT().NetBlockList(ctx)

	//stm: @CLI_NETWORK_BLOCK_LIST_001
	err := app.Run([]string{"net", "block", "list"})
	assert.NoError(t, err)
}

func TestNetUnBlockPeer(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBlockCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pid := "12D3KooWLSrbbvvvujrHrvwao6hRn1Gz8vn9eAR8QsDDKZz4xES6"
	decoded, err := peer.Decode(pid)
	assert.NoError(t, err)

	mockApi.EXPECT().NetBlockRemove(ctx, api.NetBlockList{Peers: []peer.ID{peer.ID(decoded)}})

	//stm: @CLI_NETWORK_UNBLOCK_PEER_001
	err = app.Run([]string{"net", "block", "remove", "peer", pid})
	assert.NoError(t, err)
}

func TestNetUnBlockIP(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBlockCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ip := "192.168.0.1/24"

	mockApi.EXPECT().NetBlockRemove(ctx, api.NetBlockList{IPAddrs: []string{ip}})

	//stm: @CLI_NETWORK_UNBLOCK_IP_001
	err := app.Run([]string{"net", "block", "remove", "ip", ip})
	assert.NoError(t, err)
}

func TestNetUnBlockSubnet(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("net", NetBlockCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subnet := "255.255.255.0/24"

	mockApi.EXPECT().NetBlockRemove(ctx, api.NetBlockList{IPSubnets: []string{subnet}})

	//stm: @CLI_NETWORK_UNBLOCK_SUBNET_001
	err := app.Run([]string{"net", "block", "remove", "subnet", subnet})
	assert.NoError(t, err)
}
