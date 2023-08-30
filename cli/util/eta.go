package cliutil

import (
	"fmt"
	"time"
)

// ETA implements a very simple eta calculator based on the number of remaining items. It does not
// require knowing the work size in advance and is therefore suitable for streaming workloads and
// also does not require that consecutive updates have a monotonically decreasing remaining value.
type ETA struct {
	// max number of items to keep in memory
	maxItems int
	// a queue of most recently updated items
	items []item
	// we store the last calculated ETA which we reuse if there was not change in remaining items
	lastETA string
}

type item struct {
	timestamp time.Time
	remaining int64
}

// NewETA creates a new ETA calculator of the given size
func NewETA(maxItems int) *ETA {
	return &ETA{
		maxItems: maxItems,
		items:    make([]item, 0),
	}
}

// Update updates the ETA calculator with the remaining number of items and returns the ETA
func (e *ETA) Update(remaining int64) string {
	item := item{
		timestamp: time.Now(),
		remaining: remaining,
	}

	if len(e.items) == 0 {
		e.items = append(e.items, item)
		return ""
	}

	if e.items[len(e.items)-1].remaining == remaining {
		// we ignore updates with the same remaining value and just return the previous ETA
		return e.lastETA
	} else if e.items[len(e.items)-1].remaining < remaining {
		// in case remaining went up from previous updates then we don't know how many items
		// were actually updated, lets assume 1 and update the remaining value of all items
		diff := remaining - e.items[len(e.items)-1].remaining + 1
		for i := range e.items {
			e.items[i].remaining += diff
		}
	}

	// append the item to the queue and remove the oldest item if needed
	if len(e.items) >= e.maxItems {
		e.items = e.items[1:]
	}
	e.items = append(e.items, item)

	// calculate the average processing time per item in the queue
	diffMs := e.items[len(e.items)-1].timestamp.Sub(e.items[0].timestamp).Milliseconds()
	nrItemsProcessed := e.items[0].remaining - e.items[len(e.items)-1].remaining
	avg := diffMs / nrItemsProcessed

	// use that average processing time to estimate how long the remaining items will take
	// and cache that ETA so we don't have to recalculate it on every update unless the
	// remaining value changes
	e.lastETA = msToETA(avg * remaining)

	return e.lastETA
}

func msToETA(ms int64) string {
	seconds := ms / 1000
	minutes := seconds / 60
	hours := minutes / 60

	return fmt.Sprintf("%02dh:%02dm:%02ds", hours, minutes%60, seconds%60)
}
