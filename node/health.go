package node

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	lapi "github.com/filecoin-project/lotus/api"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/network"
)

var healthlog = logging.Logger("healthcheck")

type HealthHandler struct {
	healthy int32
}

func (h *HealthHandler) SetHealthy(healthy bool) {
	var hi32 int32
	if healthy {
		hi32 = 1
	}
	atomic.StoreInt32(&h.healthy, hi32)
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&h.healthy) != 1 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Check that the node is still working. That is, that it's still processing the chain.
// If there have been no recent changes, consider the node to be dead.
func NewLiveHandler(api lapi.FullNode) *HealthHandler {
	ctx := context.Background()
	h := HealthHandler{}
	go func() {
		const reset int32 = 5
		var countdown int32 = 0
		minutely := time.NewTicker(time.Minute)
		headCh, err := api.ChainNotify(ctx)
		if err != nil {
			healthlog.Warnf("failed to instantiate chain notify channel; liveness cannot be determined. %s", err)
			h.SetHealthy(false)
			return
		}
		for {
			select {
			case <-minutely.C:
				atomic.AddInt32(&countdown, -1)
				if countdown <= 0 {
					h.SetHealthy(false)
				}
			case <-headCh:
				atomic.StoreInt32(&countdown, reset)
				h.SetHealthy(true)
			}
		}
	}()
	return &h
}

// Check if we are ready to handle traffic.
// 1. sync workers are reasonably up to date.
// 2. libp2p is servicable
func NewReadyHandler(api lapi.FullNode) *HealthHandler {
	ctx := context.Background()
	h := HealthHandler{}
	go func() {
		const heightTolerance = uint64(5)
		var nethealth, synchealth bool
		minutely := time.NewTicker(time.Minute)
		for {
			select {
			case <-minutely.C:
				netstat, err := api.NetAutoNatStatus(ctx)
				nethealth = err == nil && netstat.Reachability != network.ReachabilityUnknown

				nodestat, err := api.NodeStatus(ctx, false)
				synchealth = err == nil && nodestat.SyncStatus.Behind < heightTolerance

				h.SetHealthy(nethealth && synchealth)
			}
		}
	}()
	return &h
}
