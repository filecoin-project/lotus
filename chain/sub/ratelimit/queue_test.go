package ratelimit

import (
	"testing"
)

func TestQueue(t *testing.T) {
	const size = 3
	q := &queue{buf: make([]int64, size)}

	if q.len() != 0 {
		t.Fatalf("q.len() = %d, expect 0", q.len())
	}

	if q.cap() != size {
		t.Fatalf("q.cap() = %d, expect %d", q.cap(), size)
	}

	for i := int64(0); i < int64(size); i++ {
		err := q.push(i)
		if err != nil {
			t.Fatalf("cannot push element %d", i)
		}
	}

	if q.len() != size {
		t.Fatalf("q.len() = %d, expect %d", q.len(), size)
	}

	err := q.push(int64(size))
	if err != ErrRateLimitExceeded {
		t.Fatalf("pushing element beyond capacity should have failed with err: %s, got %s", ErrRateLimitExceeded, err)
	}

	if q.front() != 0 {
		t.Fatalf("q.front() = %d, expect 0", q.front())
	}

	if q.back() != int64(size-1) {
		t.Fatalf("q.back() = %d, expect %d", q.back(), size-1)
	}

	popVal := q.pop()
	if popVal != 0 {
		t.Fatalf("q.pop() = %d, expect 0", popVal)
	}

	if q.len() != size-1 {
		t.Fatalf("q.len() = %d, expect %d", q.len(), size-1)
	}

	// Testing truncation.
	threshold := int64(1)
	q.truncate(threshold)
	if q.len() != 1 {
		t.Fatalf("q.len() after truncate = %d, expect 1", q.len())
	}
	if q.front() != 2 {
		t.Fatalf("q.front() after truncate = %d, expect 2", q.front())
	}
}
