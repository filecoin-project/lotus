package headbuffer

import (
	"container/list"

	"github.com/filecoin-project/lotus/api"
)

type HeadChangeStackBuffer struct {
	buffer *list.List
	size   int
}

// NewHeadChangeStackBuffer buffer HeadChange events to avoid having to
// deal with revert changes. Initialized size should be the average reorg
// size + 1
func NewHeadChangeStackBuffer(size int) *HeadChangeStackBuffer {
	buffer := list.New()
	buffer.Init()

	return &HeadChangeStackBuffer{
		buffer: buffer,
		size:   size,
	}
}

// Push adds a HeadChange to stack buffer. If the length of
// the stack buffer grows larger than the initialized size, the
// oldest HeadChange is returned.
func (h *HeadChangeStackBuffer) Push(hc *api.HeadChange) (rethc *api.HeadChange) {
	if h.buffer.Len() >= h.size {
		var ok bool

		el := h.buffer.Front()
		rethc, ok = el.Value.(*api.HeadChange)
		if !ok {
			// This shouldn't be possible, this method is typed and is the only place data
			// pushed to the buffer.
			panic("A cosmic ray made me do it")
		}

		h.buffer.Remove(el)
	}

	h.buffer.PushBack(hc)

	return
}

// Pop removes the last added HeadChange
func (h *HeadChangeStackBuffer) Pop() {
	el := h.buffer.Back()
	if el != nil {
		h.buffer.Remove(el)
	}
}
