package store

import (
	"github.com/ipfs/go-cid"
)

type SnapshotVersion uint64

const (
	SnapshotVersion1 SnapshotVersion = iota + 1
	SnapshotVersion2
)
const V2SnapshotRootCount int = 1

type SnapshotMetadata struct {
	Version       SnapshotVersion
	HeadTipsetKey []cid.Cid
	F3Data        *cid.Cid
}
