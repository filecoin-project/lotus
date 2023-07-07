package sealer

import (
	"context"
	"sync"

	"github.com/filecoin-project/lotus/lib/lazy"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// schedWorkerCache caches scheduling-related calls to workers
type schedWorkerCache struct {
	Workers map[storiface.WorkerID]*WorkerHandle

	lk     sync.Mutex
	cached map[storiface.WorkerID]*cachedSchedWorker
}

func (s *schedWorkerCache) Get(id storiface.WorkerID) (*cachedSchedWorker, bool) {
	s.lk.Lock()
	defer s.lk.Unlock()

	if _, found := s.cached[id]; !found {
		if _, found := s.Workers[id]; !found {
			return nil, false
		}

		whnd := s.Workers[id]

		s.cached[id] = &cachedSchedWorker{
			tt:    lazy.MakeLazyCtx(whnd.workerRpc.TaskTypes),
			paths: lazy.MakeLazyCtx(whnd.workerRpc.Paths),
			utilization: lazy.MakeLazy(func() (float64, error) {
				return whnd.Utilization(), nil
			}),

			Enabled: whnd.Enabled,
			Info:    whnd.Info,
		}
	}

	return s.cached[id], true
}

type cachedSchedWorker struct {
	tt          *lazy.LazyCtx[map[sealtasks.TaskType]struct{}]
	paths       *lazy.LazyCtx[[]storiface.StoragePath]
	utilization *lazy.Lazy[float64]

	Enabled bool
	Info    storiface.WorkerInfo
}

func (c *cachedSchedWorker) TaskTypes(ctx context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return c.tt.Val(ctx)
}

func (c *cachedSchedWorker) Paths(ctx context.Context) ([]storiface.StoragePath, error) {
	return c.paths.Get(ctx)
}

func (c *cachedSchedWorker) Utilization() float64 {
	// can't error
	v, _ := c.utilization.Val()
	return v
}

var _ SchedWorker = &cachedSchedWorker{}
