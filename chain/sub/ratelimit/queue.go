package ratelimit

import "errors"

var ErrRateLimitExceeded = errors.New("rate limit exceeded")

type queue struct {
	buf   []int64
	count int
	head  int
	tail  int
}

// cap returns the queue capacity
func (q *queue) cap() int {
	return len(q.buf)
}

// len returns the number of items in the queue
func (q *queue) len() int {
	return q.count
}

// push adds an element to the end of the queue.
func (q *queue) push(elem int64) error {
	if q.count == len(q.buf) {
		return ErrRateLimitExceeded
	}

	q.buf[q.tail] = elem
	// Calculate new tail position.
	q.tail = q.next(q.tail)
	q.count++
	return nil
}

// pop removes and returns the element from the front of the queue.
func (q *queue) pop() int64 {
	if q.count <= 0 {
		panic("pop from empty queue")
	}
	ret := q.buf[q.head]

	// Calculate new head position.
	q.head = q.next(q.head)
	q.count--

	return ret
}

// front returns the element at the front of the queue.  This is the element
// that would be returned by pop().  This call panics if the queue is empty.
func (q *queue) front() int64 {
	if q.count <= 0 {
		panic("front() called when empty")
	}
	return q.buf[q.head]
}

// back returns the element at the back of the queue.  This call panics if the
// queue is empty.
func (q *queue) back() int64 {
	if q.count <= 0 {
		panic("back() called when empty")
	}
	return q.buf[q.prev(q.tail)]
}

// prev returns the previous buffer position wrapping around buffer.
func (q *queue) prev(i int) int {
	if i == 0 {
		return len(q.buf) - 1
	}
	return (i - 1) % len(q.buf)
}

// next returns the next buffer position wrapping around buffer.
func (q *queue) next(i int) int {
	return (i + 1) % len(q.buf)
}

// truncate pops values that are less than or equal the specified threshold.
func (q *queue) truncate(threshold int64) {
	for q.count != 0 && q.buf[q.head] <= threshold {
		// pop() without returning a value
		q.head = q.next(q.head)
		q.count--
	}
}
