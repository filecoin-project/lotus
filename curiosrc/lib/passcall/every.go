package passcall

import (
	"sync"
	"time"
)

// Every is a helper function that will call the provided callback
// function at most once every `passEvery` duration. If the function is called
// more frequently than that, it will return nil and not call the callback.
func Every[P, R any](passInterval time.Duration, cb func(P) R) func(P) R {
	var lastCall time.Time
	var lk sync.Mutex

	return func(param P) R {
		lk.Lock()
		defer lk.Unlock()

		if time.Since(lastCall) < passInterval {
			return *new(R)
		}

		defer func() {
			lastCall = time.Now()
		}()
		return cb(param)
	}
}
