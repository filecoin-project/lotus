package journal

import "time"

// EventType represents the signature of an event.
type EventType struct {
	System string
	Event  string
}

// Journal represents an audit trail of system actions.
//
// Every entry is tagged with a timestamp, a system name, and an event name.
// The supplied data can be any type, as long as it is JSON serializable,
// including structs, map[string]interface{}, or primitive types.
//
// For cleanliness and type safety, we recommend to use typed events. See the
// *Evt struct types in this package for more info.
type Journal interface {
	// IsEnabled allows components to check if a given event type is enabled.
	// All event types are enabled by default, and specific event types can only
	// be disabled at construction type. Components are advised to check if the
	// journal event types they record are enabled as soon as possible, and hold
	// on to the answer all throughout the lifetime of the process.
	IsEnabled(evtType EventType) bool

	// AddEntry adds an entry to this journal. See godocs on the Journal type
	// for more info.
	AddEntry(evtType EventType, data interface{})

	// Close closes this journal for further writing.
	Close() error
}

// Entry represents a journal entry.
//
// See godocs on Journal for more information.
type Entry struct {
	EventType

	Timestamp time.Time
	Data      interface{}
}

// disabledTracker is an embeddable mixin that takes care of tracking disabled
// event types.
type disabledTracker map[EventType]struct{}

func newDisabledTracker(disabled []EventType) disabledTracker {
	dis := make(map[EventType]struct{}, len(disabled))
	for _, et := range disabled {
		dis[et] = struct{}{}
	}
	return dis
}

func (d disabledTracker) IsEnabled(evtType EventType) bool {
	_, ok := d[evtType]
	return !ok
}
