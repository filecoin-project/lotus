package splitstore

import (
	cid "github.com/ipfs/go-cid"
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

type cidSetVisitor struct {
	set *cid.Set
}

var _ ObjectVisitor = (*cidSetVisitor)(nil)

func (v *cidSetVisitor) Visit(c cid.Cid) (bool, error) {
	return v.set.Visit(c), nil
}

func tmpVisitor() ObjectVisitor {
	return &cidSetVisitor{set: cid.NewSet()}
}
