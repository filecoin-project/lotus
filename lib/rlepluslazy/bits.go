package rlepluslazy

import (
	"sort"
)

type it2b struct {
	source RunIterator
	curIdx uint64

	run Run
}

func (it *it2b) HasNext() bool {
	return it.run.Valid()
}

func (it *it2b) Next() (uint64, error) {
	it.run.Len--
	res := it.curIdx
	it.curIdx++
	return res, it.prep()
}

func (it *it2b) prep() error {
	for !it.run.Valid() && it.source.HasNext() {
		var err error
		it.run, err = it.source.NextRun()
		if err != nil {
			return err
		}

		if !it.run.Val {
			it.curIdx += it.run.Len
			it.run.Len = 0
		}
	}
	return nil
}

func BitsFromRuns(source RunIterator) (BitIterator, error) {
	it := &it2b{source: source}
	if err := it.prep(); err != nil {
		return nil, err
	}
	return it, nil
}

type sliceIt struct {
	s []uint64
}

func (it sliceIt) HasNext() bool {
	return len(it.s) != 0
}

func (it *sliceIt) Next() (uint64, error) {
	res := it.s[0]
	it.s = it.s[1:]
	return res, nil
}

func BitsFromSlice(slice []uint64) BitIterator {
	sort.Slice(slice, func(i, j int) bool { return slice[i] < slice[j] })
	return &sliceIt{slice}
}

type it2r struct {
	source BitIterator

	runIdx uint64
	run    [2]Run
}

func (it *it2r) HasNext() bool {
	return it.run[0].Valid()
}

func (it *it2r) NextRun() (Run, error) {
	res := it.run[0]
	it.runIdx = it.runIdx + res.Len
	it.run[0], it.run[1] = it.run[1], Run{}
	return res, it.prep()
}

func (it *it2r) prep() error {
	if !it.HasNext() {
		return nil
	}
	if it.run[0].Val == false {
		it.run[1].Val = true
		it.run[1].Len = 1
		return nil
	}

	for it.source.HasNext() && !it.run[1].Valid() {
		nB, err := it.source.Next()
		if err != nil {
			return err
		}

		//fmt.Printf("runIdx: %d, run[0].Len: %d, nB: %d\n", it.runIdx, it.run[0].Len, nB)
		if it.runIdx+it.run[0].Len == nB {
			it.run[0].Len++
		} else {
			it.run[1].Len = nB - it.runIdx - it.run[0].Len
			it.run[1].Val = false
		}
	}
	return nil
}

func (it *it2r) init() error {
	if it.source.HasNext() {
		nB, err := it.source.Next()
		if err != nil {
			return err
		}
		it.run[0].Len = nB
		it.run[0].Val = false
		it.run[1].Len = 1
		it.run[1].Val = true
	}

	if !it.run[0].Valid() {
		it.run[0], it.run[1] = it.run[1], Run{}
		return it.prep()
	}
	return nil
}

func RunsFromBits(source BitIterator) (RunIterator, error) {
	it := &it2r{source: source}

	if err := it.init(); err != nil {
		return nil, err
	}
	return it, nil
}
