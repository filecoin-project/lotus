package node

import (
	"context"
	"net/http"
	"time"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/libp2p/go-libp2p-core/network"
)

type HealthHandler struct {
	healthy bool
}

func (h *HealthHandler) SetHealthy(healthy bool) {
	h.healthy = healthy
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// The backend is considered alive so long as there have been recent
// head changes. Being alive doesn't mean we are up to date, just moving.
func NewLiveHandler(api lapi.FullNode) *HealthHandler {
	ctx := context.Background()
	h := HealthHandler{}
	go func() {
		const reset = 5
		var countdown = 0
		minutely := time.NewTicker(time.Minute)
		headCh, err := api.ChainNotify(ctx)
		if err != nil {
			//TODO
		}
		for {
			select {
			case <-minutely.C:
				countdown = countdown - 1
				if countdown == 0 {
					h.SetHealthy(false)
				}
			case <-headCh:
				countdown = reset
				h.SetHealthy(true)
			}
		}
	}()
	return &h
}

// Check if we are ready to handle traffic.
// 1. sync workers are caught up.
// 2
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
