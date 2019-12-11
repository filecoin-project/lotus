package rlepluslazy

import (
	"math"

	"golang.org/x/xerrors"
)

func Sum(a, b RunIterator) (RunIterator, error) {
	it := addIt{a: a, b: b}
	it.prep()
	return &it, nil
}

type addIt struct {
	a RunIterator
	b RunIterator

	next Run

	arun Run
	brun Run
}

func (it *addIt) prep() error {
	var err error

	fetch := func() error {
		if !it.arun.Valid() && it.a.HasNext() {
			it.arun, err = it.a.NextRun()
			if err != nil {
				return err
			}
		}

		if !it.brun.Valid() && it.b.HasNext() {
			it.brun, err = it.b.NextRun()
			if err != nil {
				return err
			}
		}
		return nil
	}

	if err := fetch(); err != nil {
		return err
	}

	// one is not valid
	if !it.arun.Valid() {
		it.next = it.brun
		it.brun.Len = 0
		return nil
	}

	if !it.brun.Valid() {
		it.next = it.arun
		it.arun.Len = 0
		return nil
	}

	if !(it.arun.Val || it.brun.Val) {
		min := it.arun.Len
		if it.brun.Len < min {
			min = it.brun.Len
		}
		it.next = Run{Val: it.arun.Val, Len: min}
		it.arun.Len -= it.next.Len
		it.brun.Len -= it.next.Len
		return nil
	}

	it.next = Run{Val: true}
	// different vals, 'true' wins
	for (it.arun.Val && it.arun.Valid()) || (it.brun.Val && it.brun.Valid()) {
		min := it.arun.Len
		if it.brun.Len < min && it.brun.Valid() || !it.arun.Valid() {
			min = it.brun.Len
		}
		it.next.Len += min
		if it.arun.Valid() {
			it.arun.Len -= min
		}
		if it.brun.Valid() {
			it.brun.Len -= min
		}
		if err := fetch(); err != nil {
			return err
		}
	}

	return nil
}

func (it *addIt) HasNext() bool {
	return it.next.Valid()
}

func (it *addIt) NextRun() (Run, error) {
	next := it.next
	return next, it.prep()
}

func Count(ri RunIterator) (uint64, error) {
	var count uint64

	for ri.HasNext() {
		r, err := ri.NextRun()
		if err != nil {
			return 0, err
		}
		if r.Val {
			if math.MaxUint64-r.Len < count {
				return 0, xerrors.New("RLE+ overflows")
			}
			count += r.Len
		}
	}
	return count, nil
}
