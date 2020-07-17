package journal

import (
	"context"
	"sync/atomic"

	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/build"
)

// Control messages.
type (
	clearCtrl       struct{}
	addObserverCtrl struct {
		observer *observer
		replay   bool
	}
	rmObserverCtrl *observer
	getEntriesCtrl chan []*Entry
)

type MemJournal struct {
	disabledTracker

	entries   []*Entry
	index     map[string]map[string][]*Entry
	observers []observer

	incomingCh chan *Entry
	controlCh  chan interface{}

	state  int32 // guarded by atomic; 0=closed, 1=running.
	closed chan struct{}
}

var _ Journal = (*MemJournal)(nil)

type observer struct {
	accept map[EventType]struct{}
	ch     chan *Entry
}

func (o *observer) dispatch(entry *Entry) {
	if o.accept == nil {
		o.ch <- entry
	}
	if _, ok := o.accept[entry.EventType]; ok {
		o.ch <- entry
	}
}

func NewMemoryJournal(lc fx.Lifecycle, disabled []EventType) *MemJournal {
	m := &MemJournal{
		disabledTracker: newDisabledTracker(disabled),

		index:      make(map[string]map[string][]*Entry, 16),
		observers:  make([]observer, 0, 16),
		incomingCh: make(chan *Entry, 256),
		controlCh:  make(chan interface{}, 16),
		state:      1,
		closed:     make(chan struct{}),
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error { return m.Close() },
	})

	go m.process()

	return m
}

func (m *MemJournal) AddEntry(evtType EventType, obj interface{}) {
	entry := &Entry{
		EventType: evtType,
		Timestamp: build.Clock.Now(),
		Data:      obj,
	}

	select {
	case m.incomingCh <- entry:
	case <-m.closed:
	}
}

func (m *MemJournal) Close() error {
	if !atomic.CompareAndSwapInt32(&m.state, 1, 0) {
		// already closed.
		return nil
	}
	close(m.closed)
	return nil
}

func (m *MemJournal) Clear() {
	select {
	case m.controlCh <- clearCtrl{}:
	case <-m.closed:
	}
}

// Observe starts observing events that are recorded in the MemJournal, and
// returns a channel where new events will be sent. When replay is true, all
// entries that have been recorded prior to the observer being registered will
// be replayed. To restrict the event types this observer will sent, use the
// include argument. If no include set is passed, the observer will receive all
// events types.
func (m *MemJournal) Observe(ctx context.Context, replay bool, include ...EventType) <-chan *Entry {
	var acc map[EventType]struct{}
	if include != nil {
		acc = make(map[EventType]struct{}, 16)
		for _, et := range include {
			acc[et] = struct{}{}
		}
	}

	ch := make(chan *Entry, 256)
	o := &observer{
		accept: acc,
		ch:     ch,
	}

	// watch the context, and fire the "remove observer" control message upon
	// cancellation.
	go func() {
		<-ctx.Done()
		select {
		case m.controlCh <- rmObserverCtrl(o):
		case <-m.closed:
		}
	}()

	select {
	case m.controlCh <- addObserverCtrl{o, replay}:
	case <-m.closed:
		// we are already stopped.
		close(ch)
	}

	return ch
}

// Entries gets a snapshot of stored entries.
func (m *MemJournal) Entries() []*Entry {
	ch := make(chan []*Entry)
	m.controlCh <- getEntriesCtrl(ch)
	return <-ch
}

func (m *MemJournal) process() {
	processCtrlMsg := func(message interface{}) {
		switch msg := message.(type) {
		case addObserverCtrl:
			// adding an observer.
			m.observers = append(m.observers, *msg.observer)

			if msg.replay {
				// replay all existing entries.
				for _, e := range m.entries {
					msg.observer.dispatch(e)
				}
			}
		case rmObserverCtrl:
			// removing an observer; find the observer, close its channel.
			// then discard it from our list by replacing it with the last
			// observer and reslicing.
			for i, o := range m.observers {
				if o.ch == msg.ch {
					close(o.ch)
					m.observers[i] = m.observers[len(m.observers)-1]
					m.observers = m.observers[:len(m.observers)-1]
				}
			}
		case clearCtrl:
			m.entries = m.entries[0:0]
			// carry over system and event names; there are unlikely to change;
			// just reslice the entry slices, so we are not thrashing memory.
			for _, events := range m.index {
				for ev := range events {
					events[ev] = events[ev][0:0]
				}
			}
		case getEntriesCtrl:
			cpy := make([]*Entry, len(m.entries))
			copy(cpy, m.entries)
			msg <- cpy
			close(msg)
		}
	}

	processClose := func() {
		m.entries = nil
		m.index = make(map[string]map[string][]*Entry, 16)
		for _, o := range m.observers {
			close(o.ch)
		}
		m.observers = nil
	}

	for {
		// Drain all control messages first!
		select {
		case msg := <-m.controlCh:
			processCtrlMsg(msg)
			continue
		case <-m.closed:
			processClose()
			return
		default:
		}

		// Now consume and pipe messages.
		select {
		case entry := <-m.incomingCh:
			m.entries = append(m.entries, entry)
			events := m.index[entry.System]
			if events == nil {
				events = make(map[string][]*Entry, 16)
				m.index[entry.System] = events
			}

			entries := events[entry.Event]
			events[entry.Event] = append(entries, entry)

			for _, o := range m.observers {
				o.dispatch(entry)
			}

		case msg := <-m.controlCh:
			processCtrlMsg(msg)
			continue

		case <-m.closed:
			processClose()
			return
		}
	}
}
