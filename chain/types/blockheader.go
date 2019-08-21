package types

import (
	"bytes"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	"github.com/filecoin-project/go-lotus/chain/address"
)

type Ticket struct {
	VRFProof  []byte
	VDFResult []byte
	VDFProof  []byte
}

type ElectionProof []byte

type BlockHeader struct {
	Miner address.Address

	Tickets []*Ticket

	ElectionProof []byte

	Parents []cid.Cid

	ParentWeight BigInt

	Height uint64

	StateRoot cid.Cid

	Messages cid.Cid

	BLSAggregate Signature

	MessageReceipts cid.Cid
}

type MsgMeta struct {
	BlsMessages   cid.Cid
	SecpkMessages cid.Cid
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

/*
func (blk *BlockHeader) MarshalCBOR(w io.Writer) error {
	panic("no")
}

func (blk *BlockHeader) UnmarshalCBOR(r io.Reader) error {
	panic("no")
}

func (blk *Message) MarshalCBOR(w io.Writer) error {
	panic("no")
}

func (blk *Message) UnmarshalCBOR(r io.Reader) error {
	panic("no")
}

func (blk *SignedMessage) MarshalCBOR(w io.Writer) error {
	panic("no")
}

func (blk *SignedMessage) UnmarshalCBOR(r io.Reader) error {
	panic("no")
}
*/
