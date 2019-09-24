package rlepluslazy

import (
	"errors"
	"fmt"

	"golang.org/x/xerrors"
)

const Version = 0

var (
	ErrWrongVersion = errors.New("invalid RLE+ version")
	ErrDecode       = fmt.Errorf("invalid encoding for RLE+ version %d", Version)
)

type RLE struct {
	buf []byte

	changes []change
}

type change struct {
	set   bool
	reset bool
	index uint64
}

func (c change) valid() bool {
	return c.reset || c.set
}

func FromBuf(buf []byte) (*RLE, error) {
	rle := &RLE{buf: buf}

	if len(buf) > 0 && buf[0]&3 != Version {
		return nil, xerrors.Errorf("could not create RLE+ for a buffer: %w", ErrWrongVersion)
	}

	return rle, nil
}

func (rle *RLE) RunIterator() (RunIterator, error) {
	source, err := DecodeRLE(rle.buf)
	return source, err
}

/*
	if err != nil {
		return nil, err
	}

	if len(rle.changes) == 0 {
		return source, nil
	}

	ch := rle.changes
	// using stable sort because we want to preserve change order
	sort.SliceStable(ch, func(i int, j int) bool {
		return ch[i].index < ch[j].index
	})
	for i := range ch {
		if i+1 >= len(ch) {
			break
		}
		if ch[i].index == ch[i+1].index {
			ch[i].set = false
			ch[i].reset = false
		}
	}

	sets := make([]uint64, 0, len(ch)/2)
	clears := make([]uint64, 0, len(ch)/2)
	for _, c := range ch {
		if c.set {
			sets = append(sets, c.index)
		} else if c.reset {
			clears = append(clears, c.index)
		}
	}

	setRuns, err := RunsFromSlice(sets)
	if err != nil {
		return nil, err
	}
	clearRuns, err := RunsFromSlice(clears)
	if err != nil {
		return nil, err
	}

	it := &chit{source: source, sets: setRuns, clears: clearRuns}
	return nil, nil
}

type chit struct {
	source RunIterator
	sets   RunIterator
	clears RunIterator

	next Run

	nextSource Run
	nextSet    Run
	nextClear  Run
}

func (it *chit) prep() error {
	var err error
	if !it.nextSource.Valid() && it.source.HasNext() {
		it.nextSource, err = it.source.NextRun()
		if err != nil {
			return err
		}
	}

	if !it.nextSet.Valid() && it.sets.HasNext() {
		it.nextSet, err = it.sets.NextRun()
		if err != nil {
			return err
		}
	}

	if !it.nextClear.Valid() && it.clears.HasNext() {
		it.nextClear, err = it.clears.NextRun()
		if err != nil {
			return err
		}
	}

	for len(it.changes) != 0 && !it.changes[0].valid() {
		it.changes = it.changes[1:]
	}

	var ch change
	if len(it.changes) != 0 {
		ch = it.changes[0]
	}

	if it.source.HasNext() {
		var err error
		it.nextRun, err = it.source.NextRun()
		if err != nil {
			return err
		}
	}

	if ch.valid() && ch.index < it.runIndex+it.nextRun.Len {
		if ch.set != it.nextRun.Val {
			// split run
			whole := it.nextRun
			it.nextRun.Len = ch.index - it.runIndex

		}

		// if we are here then change was valid so len(it.changes) != 0
		it.changes = it.changes[1:]
	} else {
		it.runIndex = it.runIndex + it.nextRun.Len
	}
	return nil
}

func (it *chit) HasNext() bool {
	return it.nextRun.Valid()
}

func (it *chit) NextRun() (Run, error) {
	return it.nextRun, it.prep()
}

func (rle *RLE) Set(index uint64) {
	rle.changes = append(rle.changes, change{set: true, index: index})
}

func (rle *RLE) Clear(index uint64) {
	rle.changes = append(rle.changes, change{reset: true, index: index})
}

func (rle *RLE) Merge(other *RLE) RunIterator {
	return nil
}
*/
