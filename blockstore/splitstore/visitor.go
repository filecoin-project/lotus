package splitstore

import (
	"sync"

	"github.com/ipfs/go-cid"
)

// ObjectVisitor is an interface for deduplicating objects during walks
type ObjectVisitor interface {
	Visit(cid.Cid) (bool, error)
}

type noopVisitor struct{}

var _ ObjectVisitor = (*noopVisitor)(nil)

func (v *noopVisitor) Visit(_ cid.Cid) (bool, error) {
	return true, nil
}

type tmpVisitor struct {
	set *cid.Set
}

var _ ObjectVisitor = (*tmpVisitor)(nil)

func (v *tmpVisitor) Visit(c cid.Cid) (bool, error) {
	if isUnitaryObject(c) {
		return false, nil
	}

	return v.set.Visit(c), nil
}

func newTmpVisitor() ObjectVisitor {
	return &tmpVisitor{set: cid.NewSet()}
}

type concurrentVisitor struct {
	mx  sync.Mutex
	set *cid.Set
}

var _ ObjectVisitor = (*concurrentVisitor)(nil)

func newConcurrentVisitor() *concurrentVisitor {
	return &concurrentVisitor{set: cid.NewSet()}
}

func (v *concurrentVisitor) Visit(c cid.Cid) (bool, error) {
	if isUnitaryObject(c) {
		return false, nil
	}

	v.mx.Lock()
	defer v.mx.Unlock()

	return v.set.Visit(c), nil
}
