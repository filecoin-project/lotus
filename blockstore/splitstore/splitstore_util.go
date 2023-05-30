package splitstore

import (
	"encoding/binary"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
)

func epochToBytes(epoch abi.ChainEpoch) []byte {
	return uint64ToBytes(uint64(epoch))
}

func bytesToEpoch(buf []byte) abi.ChainEpoch {
	return abi.ChainEpoch(bytesToUint64(buf))
}

func int64ToBytes(i int64) []byte {
	return uint64ToBytes(uint64(i))
}

func bytesToInt64(buf []byte) int64 {
	return int64(bytesToUint64(buf))
}

func uint64ToBytes(i uint64) []byte {
	buf := make([]byte, 16)
	n := binary.PutUvarint(buf, i)
	return buf[:n]
}

func bytesToUint64(buf []byte) uint64 {
	i, _ := binary.Uvarint(buf)
	return i
}

func isUnitaryObject(c cid.Cid) bool {
	pre := c.Prefix()
	switch pre.Codec {
	case cid.FilCommitmentSealed, cid.FilCommitmentUnsealed:
		return true
	default:
		return pre.MhType == mh.IDENTITY
	}
}

func isIdentiyCid(c cid.Cid) bool {
	return c.Prefix().MhType == mh.IDENTITY
}

func decodeIdentityCid(c cid.Cid) ([]byte, error) {
	dmh, err := mh.Decode(c.Hash())
	if err != nil {
		return nil, xerrors.Errorf("error decoding identity cid %s: %w", c, err)
	}

	// sanity check
	if dmh.Code != mh.IDENTITY {
		return nil, xerrors.Errorf("error decoding identity cid %s: hash type is not identity", c)
	}

	return dmh.Digest, nil
}
