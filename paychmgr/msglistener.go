package paychmgr

import (
	"sync"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
)

type msgListener struct {
	id string
	cb func(c cid.Cid, err error)
}

type msgListeners struct {
	lk        sync.Mutex
	listeners []*msgListener
}

func (ml *msgListeners) onMsg(mcid cid.Cid, cb func(error)) string {
	ml.lk.Lock()
	defer ml.lk.Unlock()

	l := &msgListener{
		id: uuid.New().String(),
		cb: func(c cid.Cid, err error) {
			if mcid.Equals(c) {
				cb(err)
			}
		},
	}
	ml.listeners = append(ml.listeners, l)
	return l.id
}

func (ml *msgListeners) fireMsgComplete(mcid cid.Cid, err error) {
	ml.lk.Lock()
	defer ml.lk.Unlock()

	for _, l := range ml.listeners {
		l.cb(mcid, err)
	}
}

func (ml *msgListeners) unsubscribe(sub string) {
	ml.lk.Lock()
	defer ml.lk.Unlock()

	for i, l := range ml.listeners {
		if l.id == sub {
			ml.removeListener(i)
			return
		}
	}
}

func (ml *msgListeners) removeListener(i int) {
	copy(ml.listeners[i:], ml.listeners[i+1:])
	ml.listeners[len(ml.listeners)-1] = nil
	ml.listeners = ml.listeners[:len(ml.listeners)-1]
}
