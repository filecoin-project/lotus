package node

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/network"

	lapi "github.com/filecoin-project/lotus/api"
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

// NewLiveHandler checks that the node is still working. That is, that it's still processing the chain.
// If there have been no recent changes, consider the node to be dead.
func NewLiveHandler(api lapi.FullNode) *HealthHandler {
	ctx := context.Background()
	h := HealthHandler{}
	go func() {
		const (
			reset      int32         = 5
			maxbackoff time.Duration = time.Minute
			minbackoff time.Duration = time.Second
		)
		var (
			countdown int32
			headCh    <-chan []*lapi.HeadChange
			backoff   = minbackoff
			err       error
		)
		minutely := time.NewTicker(time.Minute)
		for {
			if headCh == nil {
				healthlog.Infof("waiting %v before starting ChainNotify channel", backoff)
				<-time.After(backoff)
				headCh, err = api.ChainNotify(ctx)
				if err != nil {
					healthlog.Warnf("failed to instantiate ChainNotify channel; cannot determine liveness. %s", err)
					h.SetHealthy(false)
					nextbackoff := 2 * backoff
					if nextbackoff > maxbackoff {
						nextbackoff = maxbackoff
					}
					backoff = nextbackoff
					continue
				}
				healthlog.Infof("started ChainNotify channel")
				backoff = minbackoff
			}
			select {
			case <-minutely.C:
				atomic.AddInt32(&countdown, -1)
				if countdown <= 0 {
					h.SetHealthy(false)
				}
			case _, ok := <-headCh:
				if !ok { // channel is closed, enter reconnect loop.
					h.SetHealthy(false)
					headCh = nil
					continue
				}
				atomic.StoreInt32(&countdown, reset)
				h.SetHealthy(true)
			}
		}
	}()
	return &h
}

// NewReadyHandler checks if we are ready to handle traffic.
// 1. sync workers are reasonably up to date.
// 2. libp2p is serviceable
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
