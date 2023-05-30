package sealer

import "sort"

type RequestQueue []*WorkerRequest

func (q RequestQueue) Len() int { return len(q) }

func (q RequestQueue) Less(i, j int) bool {
	oneMuchLess, muchLess := q[i].TaskType.MuchLess(q[j].TaskType)
	if oneMuchLess {
		return muchLess
	}

	if q[i].Priority != q[j].Priority {
		return q[i].Priority > q[j].Priority
	}

	if q[i].TaskType != q[j].TaskType {
		return q[i].TaskType.Less(q[j].TaskType)
	}

	return q[i].Sector.ID.Number < q[j].Sector.ID.Number // optimize minerActor.NewSectors bitfield
}

func (q RequestQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *RequestQueue) Push(x *WorkerRequest) {
	n := len(*q)
	item := x
	item.index = n
	*q = append(*q, item)
	sort.Sort(q)
}

func (q *RequestQueue) Remove(i int) *WorkerRequest {
	old := *q
	n := len(old)
	item := old[i]
	old[i] = old[n-1]
	old[n-1] = nil
	item.index = -1
	*q = old[0 : n-1]
	sort.Sort(q)
	return item
}
