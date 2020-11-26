package splitstore

import (
	cid "github.com/ipfs/go-cid"
)

type LiveSet interface {
	Mark(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Close() error
}
