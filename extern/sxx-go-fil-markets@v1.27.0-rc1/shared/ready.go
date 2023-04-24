package shared

import (
	"context"
	"errors"
	"sync"

	"github.com/hannahhoward/go-pubsub"
)

// ReadyFunc is function that gets called once when an event is ready
type ReadyFunc func(error)

// ReadyDispatcher is just an pubsub dispatcher where the callback is ReadyFunc
func ReadyDispatcher(evt pubsub.Event, fn pubsub.SubscriberFn) error {
	migrateErr, ok := evt.(error)
	if !ok && evt != nil {
		return errors.New("wrong type of event")
	}
	cb, ok := fn.(ReadyFunc)
	if !ok {
		return errors.New("wrong type of event")
	}
	cb(migrateErr)
	return nil
}

// ReadyManager managers listeners for a ready event
type ReadyManager struct {
	ctx  context.Context
	Stop context.CancelFunc

	lk      sync.RWMutex
	isReady bool
	initErr error
	pubsub  *pubsub.PubSub
}

func NewReadyManager() *ReadyManager {
	ctx, stop := context.WithCancel(context.Background())
	return &ReadyManager{
		ctx:    ctx,
		Stop:   stop,
		pubsub: pubsub.New(ReadyDispatcher),
	}
}

// FireReady is called when the ready event occurs
func (m *ReadyManager) FireReady(err error) error {
	m.lk.Lock()
	defer m.lk.Unlock()

	if m.isReady {
		return nil
	}

	m.isReady = true
	m.initErr = err
	return m.pubsub.Publish(err)
}

// OnReady registers a listener for the ready event.
// If the event has already been fired, the callback is immediately called back
// (in a go-routine).
func (m *ReadyManager) OnReady(ready ReadyFunc) {
	m.lk.Lock()
	defer m.lk.Unlock()

	if m.isReady {
		initErr := m.initErr
		go ready(initErr)
		return
	}

	m.pubsub.Subscribe(ready)
}

// AwaitReady blocks until the ready event fires.
// Returns immediately if the event already fired.
func (m *ReadyManager) AwaitReady() error {
	m.lk.RLock()
	isReady := m.isReady
	m.lk.RUnlock()

	if isReady {
		return m.initErr
	}

	errch := make(chan error)
	m.OnReady(func(err error) {
		select {
		case <-m.ctx.Done():
			errch <- m.ctx.Err()
		case errch <- err:
		}
	})

	select {
	case <-m.ctx.Done():
		return m.ctx.Err()
	case err := <-errch:
		return err
	}
}
