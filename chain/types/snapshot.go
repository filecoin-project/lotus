package types

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
	Version        SnapshotVersion
	HeadTipsetKeys []cid.Cid
	F3Data         *cid.Cid
}
