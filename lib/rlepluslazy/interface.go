package rlepluslazy

type Run struct {
	Val bool
	Len uint64
}

func (r Run) Valid() bool {
	return r.Len != 0
}

type RunIterator interface {
	NextRun() (Run, error)
	HasNext() bool
}

type RunIterable interface {
	RunIterator() (RunIterator, error)
}

type BitIterator interface {
	Next() (uint64, error)
	HasNext() bool
}
