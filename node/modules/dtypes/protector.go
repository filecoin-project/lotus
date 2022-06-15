package dtypes

import (
	"github.com/ipfs/go-cid"
)

type GCReferenceProtector interface {
	AddProtector(func(func(cid.Cid) error) error)
}

type NoopGCReferenceProtector struct{}

func (p NoopGCReferenceProtector) AddProtector(func(func(cid.Cid) error) error) {}
