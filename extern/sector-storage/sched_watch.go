package sectorstorage

import (
	"context"
	"reflect"
)

func (sh *scheduler) runWorkerWatcher() {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	nilch := reflect.ValueOf(new(chan struct{})).Elem()

	cases := []reflect.SelectCase{
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(sh.closing),
		},
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(sh.watchClosing),
		},
	}

	caseToWorker := map[int]WorkerID{}

	for {
		n, rv, ok := reflect.Select(cases)

		switch {
		case n == 0: // sh.closing
			return
		case n == 1: // sh.watchClosing
			if !ok {
				log.Errorf("watchClosing channel closed")
				return
			}

			wid, ok := rv.Interface().(WorkerID)
			if !ok {
				panic("got a non-WorkerID message")
			}

			sh.workersLk.Lock()
			workerClosing, err := sh.workers[wid].w.Closing(ctx)
			sh.workersLk.Unlock()
			if err != nil {
				log.Errorf("getting worker closing channel: %+v", err)
				select {
				case sh.workerClosing <- wid:
				case <-sh.closing:
					return
				}

				continue
			}

			toSet := -1
			for i, sc := range cases {
				if sc.Chan == nilch {
					toSet = i
					break
				}
			}
			if toSet == -1 {
				toSet = len(cases)
				cases = append(cases, reflect.SelectCase{})
			}

			cases[toSet] = reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(workerClosing),
			}

			caseToWorker[toSet] = wid
		default:
			wid, found := caseToWorker[n]
			if !found {
				log.Errorf("worker ID not found for case %d", n)
				continue
			}

			delete(caseToWorker, n)
			cases[n] = reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: nilch,
			}

			log.Warnf("worker %d dropped", wid)
			// send in a goroutine to avoid a deadlock between workerClosing / watchClosing
			go func() {
				select {
				case sh.workerClosing <- wid:
				case <-sh.closing:
					return
				}
			}()
		}
	}
}
