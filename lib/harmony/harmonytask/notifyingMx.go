package harmonytask

import "sync"

type notifyingMx struct {
	sync.Mutex
	UnlockNotify func()
}

func (n *notifyingMx) Unlock() {
	tmp := n.UnlockNotify
	n.Mutex.Unlock()
	if tmp != nil {
		tmp()
	}
}
