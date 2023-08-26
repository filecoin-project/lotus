package cliutil

import (
	"fmt"
	"time"
)

// ETA is a very simple ETA calculator
type ETA struct {
	// how many items in buff we use for calculating ETA
	size int
	// buffer of processing time of past size updates
	buff []item
	// we store the last calculated ETA which we reuse if there was not change in remaining items
	cachedETA string
}

type item struct {
	timestamp time.Time
	remaining int64
}

// NewETA creates a new ETA calculator with the given buffer size
func NewETA(size int) *ETA {
	return &ETA{
		size: size,
		buff: make([]item, 0),
	}
}

// Update updates the ETA calculator with the remaining number of items and returns the ETA
func (e *ETA) Update(remaining int64) string {
	item := item{
		timestamp: time.Now(),
		remaining: remaining,
	}

	if len(e.buff) == 0 {
		e.buff = append(e.buff, item)
		return ""
	}

	// we ignore updates with the same remaining value and just return the previous ETA
	if e.buff[len(e.buff)-1].remaining == remaining {
		return e.cachedETA
	}

	// remove oldest item from buffer if its full
	if len(e.buff) >= e.size {
		e.buff = e.buff[1:]
	}

	e.buff = append(e.buff, item)

	// calculate the average processing time per item in the buffer
	diffMs := e.buff[len(e.buff)-1].timestamp.Sub(e.buff[0].timestamp).Milliseconds()
	avg := diffMs / int64(len(e.buff))

	// use that average processing time to estimate how long the remaining items will take
	// and cache that ETA so we don't have to recalculate it on every update unless the
	// remaining value changes
	e.cachedETA = msToETA(avg * remaining)

	return e.cachedETA
}

func msToETA(ms int64) string {
	seconds := ms / 1000
	sec := seconds % 60
	minutes := seconds / 60
	min := minutes % 60
	hour := minutes / 60
	return fmt.Sprintf("%02dh:%02dm:%02ds", hour, min, sec)
}
