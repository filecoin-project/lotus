package types

import (
	"bytes"
	"math/big"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/minio/sha256-simd"
	"github.com/multiformats/go-multihash"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
)

type Ticket struct {
	VRFProof []byte
}

type EPostTicket struct {
	Partial        []byte
	SectorID       abi.SectorNumber
	ChallengeIndex uint64
}

type EPostProof struct {
	Proofs     []abi.PoStProof
	PostRand   []byte
	Candidates []EPostTicket
}

type BlockHeader struct {
	Miner address.Address // 0

	Ticket *Ticket // 1

	EPostProof EPostProof // 2

	Parents []cid.Cid // 3

	ParentWeight BigInt // 4

	Height abi.ChainEpoch // 5

	ParentStateRoot cid.Cid // 6

	ParentMessageReceipts cid.Cid // 7

	Messages cid.Cid // 8

	BLSAggregate crypto.Signature // 9

	Timestamp uint64 // 10

	BlockSig *crypto.Signature // 11

	ForkSignaling uint64 // 12
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

var blocksPerEpoch = NewInt(build.BlocksPerEpoch)

const sha256bits = 256

func IsTicketWinner(partialTicket []byte, ssizeI abi.SectorSize, snum uint64, totpow BigInt) bool {
	ssize := NewInt(uint64(ssizeI))
	ssampled := ElectionPostChallengeCount(snum, 0) // TODO: faults in epost?
	/*
		Need to check that
		(h(vrfout) + 1) / (max(h) + 1) <= e * sectorSize / totalPower
		max(h) == 2^256-1
		which in terms of integer math means:
		(h(vrfout) + 1) * totalPower <= e * sectorSize * 2^256
		in 2^256 space, it is equivalent to:
		h(vrfout) * totalPower < e * sectorSize * 2^256

		Because of SectorChallengeRatioDiv sampling for proofs
		we need to scale this appropriately.

		Let c = ceil(numSectors/SectorChallengeRatioDiv)
		(c is the number of tickets a miner requests)
		Accordingly we check
		(h(vrfout) + 1) / 2^256 <= e * sectorSize / totalPower * snum / c
		or
		h(vrfout) * totalPower * c < e * sectorSize * 2^256 * snum
	*/

	h := sha256.Sum256(partialTicket)

	lhs := BigFromBytes(h[:]).Int
	lhs = lhs.Mul(lhs, totpow.Int)
	lhs = lhs.Mul(lhs, new(big.Int).SetUint64(ssampled))

	// rhs = sectorSize * 2^256
	// rhs = sectorSize << 256
	rhs := new(big.Int).Lsh(ssize.Int, sha256bits)
	rhs = rhs.Mul(rhs, new(big.Int).SetUint64(snum))
	rhs = rhs.Mul(rhs, blocksPerEpoch.Int)

	// h(vrfout) * totalPower < e * sectorSize * 2^256?
	return lhs.Cmp(rhs) < 0
}

func ElectionPostChallengeCount(sectors uint64, faults uint64) uint64 {
	if sectors-faults == 0 {
		return 0
	}
	// ceil(sectors / SectorChallengeRatioDiv)
	return (sectors-faults-1)/build.SectorChallengeRatioDiv + 1
}

func (t *Ticket) Equals(ot *Ticket) bool {
	return bytes.Equal(t.VRFProof, ot.VRFProof)
}
