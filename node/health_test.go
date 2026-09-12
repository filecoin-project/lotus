package node

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/mocks"
)

func TestNewReadyHandlerChecksImmediately(t *testing.T) {
	ctrl := gomock.NewController(t)
	fullNode := mocks.NewMockFullNode(ctrl)
	fullNode.EXPECT().NetAutoNatStatus(gomock.Any()).Return(api.NatInfo{
		Reachability: network.ReachabilityPublic,
	}, nil).AnyTimes()
	fullNode.EXPECT().NodeStatus(gomock.Any(), false).Return(api.NodeStatus{
		SyncStatus: api.NodeSyncStatus{Behind: 0},
	}, nil).AnyTimes()

	handler := NewReadyHandler(fullNode)

	require.Eventually(t, func() bool {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/readyz", nil))
		return response.Code == http.StatusOK
	}, time.Second, 10*time.Millisecond)
}
