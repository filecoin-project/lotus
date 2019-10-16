package types

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math/big"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"go.opencensus.io/trace"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/address"
)

type Ticket struct {
	VRFProof []byte
}

type ElectionProof []byte

type BlockHeader struct {
	Miner address.Address

	Tickets []*Ticket

	ElectionProof []byte

	Parents []cid.Cid

	ParentWeight BigInt

	Height uint64

	ParentStateRoot cid.Cid

	ParentMessageReceipts cid.Cid

	Messages cid.Cid

	BLSAggregate Signature

	Timestamp uint64

	BlockSig Signature
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
		panic(err)
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
	return blk.Tickets[len(blk.Tickets)-1]
}

func (blk *BlockHeader) SigningBytes() ([]byte, error) {
	blkcopy := *blk
	blkcopy.BlockSig = Signature{}

	return blkcopy.Serialize()
}

func (blk *BlockHeader) CheckBlockSignature(ctx context.Context, worker address.Address) error {
	ctx, span := trace.StartSpan(ctx, "checkBlockSignature")
	defer span.End()

	sigb, err := blk.SigningBytes()
	if err != nil {
		return xerrors.Errorf("failed to get block signing bytes: %w", err)
	}

	return blk.BlockSig.Verify(worker, sigb)
}

type MsgMeta struct {
	BlsMessages   cid.Cid
	SecpkMessages cid.Cid
}

func (mm *MsgMeta) Cid() cid.Cid {
	b, err := mm.ToStorageBlock()
	if err != nil {
		panic(err)
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

func PowerCmp(eproof ElectionProof, mpow, totpow BigInt) bool {

	/*
		Need to check that
		h(vrfout) / 2^256 < minerPower / totalPower
	*/

	h := sha256.Sum256(eproof)

	// 2^256
	rden := BigInt{big.NewInt(0).Exp(big.NewInt(2), big.NewInt(256), nil)}

	top := BigMul(rden, mpow)
	out := BigDiv(top, totpow)

	hp := BigFromBytes(h[:])
	return hp.LessThan(out)
}

func (t *Ticket) Equals(ot *Ticket) bool {
	return bytes.Equal(t.VRFProof, ot.VRFProof)
}
