package types

import (
	"bytes"
	"math/big"

	"github.com/minio/blake2b-simd"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
)

type Ticket struct {
	VRFProof []byte
}

type ElectionProof struct {
	VRFProof []byte
}

type BeaconEntry struct {
	Round uint64
	Data  []byte
}

func NewBeaconEntry(round uint64, data []byte) BeaconEntry {
	return BeaconEntry{
		Round: round,
		Data:  data,
	}
}

type BlockHeader struct {
	Miner address.Address // 0

	Ticket *Ticket // 1

	ElectionProof *ElectionProof // 2

	BeaconEntries []BeaconEntry // 3

	WinPoStProof []abi.PoStProof // 4

	Parents []cid.Cid // 5

	ParentWeight BigInt // 6

	Height abi.ChainEpoch // 7

	ParentStateRoot cid.Cid // 8

	ParentMessageReceipts cid.Cid // 8

	Messages cid.Cid // 10

	BLSAggregate *crypto.Signature // 11

	Timestamp uint64 // 12

	BlockSig *crypto.Signature // 13

	ForkSignaling uint64 // 14

	// internal
	validated bool // true if the signature has been validated
}

func (b *BlockHeader) ToStorageBlock() (block.Block, error) {
	data, err := b.Serialize()
	if err != nil {
		return nil, err
	}

	pref := cid.NewPrefixV1(cid.DagCBOR, multihash.BLAKE2B_MIN+31)
	c, err := pref.Sum(data)
	if err != nil {
		return nil, err
	}

	return block.NewBlockWithCid(data, c)
}

func (b *BlockHeader) Cid() cid.Cid {
	sb, err := b.ToStorageBlock()
	if err != nil {
		panic(err) // Not sure i'm entirely comfortable with this one, needs to be checked
	}

	return sb.Cid()
}

func DecodeBlock(b []byte) (*BlockHeader, error) {
	var blk BlockHeader
	if err := blk.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, err
	}

	return &blk, nil
}

func (blk *BlockHeader) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := blk.MarshalCBOR(buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (blk *BlockHeader) LastTicket() *Ticket {
	return blk.Ticket
}

func (blk *BlockHeader) SigningBytes() ([]byte, error) {
	blkcopy := *blk
	blkcopy.BlockSig = nil

	return blkcopy.Serialize()
}

func (blk *BlockHeader) SetValidated() {
	blk.validated = true
}

func (blk *BlockHeader) IsValidated() bool {
	return blk.validated
}

type MsgMeta struct {
	BlsMessages   cid.Cid
	SecpkMessages cid.Cid
}

func (mm *MsgMeta) Cid() cid.Cid {
	b, err := mm.ToStorageBlock()
	if err != nil {
		panic(err) // also maybe sketchy
	}
	return b.Cid()
}

func (mm *MsgMeta) ToStorageBlock() (block.Block, error) {
	buf := new(bytes.Buffer)
	if err := mm.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal MsgMeta: %w", err)
	}

	pref := cid.NewPrefixV1(cid.DagCBOR, multihash.BLAKE2B_MIN+31)
	c, err := pref.Sum(buf.Bytes())
	if err != nil {
		return nil, err
	}

	return block.NewBlockWithCid(buf.Bytes(), c)
}

func CidArrsEqual(a, b []cid.Cid) bool {
	if len(a) != len(b) {
		return false
	}

	// order ignoring compare...
	s := make(map[cid.Cid]bool)
	for _, c := range a {
		s[c] = true
	}

	for _, c := range b {
		if !s[c] {
			return false
		}
	}
	return true
}

func CidArrsContains(a []cid.Cid, b cid.Cid) bool {
	for _, elem := range a {
		if elem.Equals(b) {
			return true
		}
	}
	return false
}

var blocksPerEpoch = NewInt(build.BlocksPerEpoch)

const sha256bits = 256

func IsTicketWinner(vrfTicket []byte, mypow BigInt, totpow BigInt) bool {
	/*
		Need to check that
		(h(vrfout) + 1) / (max(h) + 1) <= e * myPower / totalPower
		max(h) == 2^256-1
		which in terms of integer math means:
		(h(vrfout) + 1) * totalPower <= e * myPower * 2^256
		in 2^256 space, it is equivalent to:
		h(vrfout) * totalPower < e * myPower * 2^256

	*/

	h := blake2b.Sum256(vrfTicket)

	lhs := BigFromBytes(h[:]).Int
	lhs = lhs.Mul(lhs, totpow.Int)

	// rhs = sectorSize * 2^256
	// rhs = sectorSize << 256
	rhs := new(big.Int).Lsh(mypow.Int, sha256bits)
	rhs = rhs.Mul(rhs, blocksPerEpoch.Int)

	// h(vrfout) * totalPower < e * sectorSize * 2^256?
	return lhs.Cmp(rhs) < 0
}

func (t *Ticket) Equals(ot *Ticket) bool {
	return bytes.Equal(t.VRFProof, ot.VRFProof)
}
