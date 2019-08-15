package types

import (
	"fmt"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
	"github.com/polydawn/refmt/obj/atlas"

	"github.com/filecoin-project/go-lotus/chain/address"
)

func init() {
	cbor.RegisterCborType(atlas.BuildEntry(BlockHeader{}).UseTag(43).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(blk BlockHeader) ([]interface{}, error) {
				if blk.Tickets == nil {
					blk.Tickets = []*Ticket{}
				}
				if blk.Parents == nil {
					blk.Parents = []cid.Cid{}
				}

				var tickarrs [][][]byte // oh boy
				for _, t := range blk.Tickets {
					tickarrs = append(tickarrs, [][]byte{
						t.VRFProof, t.VDFResult, t.VDFProof,
					})
				}

				return []interface{}{
					blk.Miner.Bytes(),
					tickarrs,
					blk.ElectionProof,
					blk.Parents,
					blk.ParentWeight,
					blk.Height,
					blk.StateRoot,
					blk.Messages,
					blk.MessageReceipts,
				}, nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(arr []interface{}) (BlockHeader, error) {
				miner, err := address.NewFromBytes(arr[0].([]byte))
				if err != nil {
					return BlockHeader{}, err
				}

				tickets := []*Ticket{}
				ticketarr, _ := arr[1].([]interface{})
				for _, t := range ticketarr {
					ticklist, ok := t.([]interface{})
					if !ok {
						return BlockHeader{}, fmt.Errorf("tickets were incorrectly formatted (type = %T)", t)
					}

					if len(ticklist) != 3 {
						return BlockHeader{}, fmt.Errorf("ticket should be a three item array of Byte arrays (got len = %d)", len(ticklist))
					}

					tickets = append(tickets, &Ticket{
						VRFProof:  ticklist[0].([]byte),
						VDFResult: ticklist[1].([]byte),
						VDFProof:  ticklist[2].([]byte),
					})
				}
				electionProof, _ := arr[2].([]byte)

				parents := []cid.Cid{}
				parentsArr, _ := arr[3].([]interface{})
				for _, p := range parentsArr {
					parents = append(parents, p.(cid.Cid))
				}
				parentWeight := arr[4].(BigInt)
				height := arr[5].(uint64)
				stateRoot := arr[6].(cid.Cid)

				msgscid := arr[7].(cid.Cid)
				recscid := arr[8].(cid.Cid)

				return BlockHeader{
					Miner:           miner,
					Tickets:         tickets,
					ElectionProof:   electionProof,
					Parents:         parents,
					ParentWeight:    parentWeight,
					Height:          height,
					StateRoot:       stateRoot,
					Messages:        msgscid,
					MessageReceipts: recscid,
				}, nil
			})).
		Complete())
	cbor.RegisterCborType(MsgMeta{})
	cbor.RegisterCborType(Ticket{})
}

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

	pref := cid.NewPrefixV1(0x1f, multihash.BLAKE2B_MIN+31)
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
	if err := cbor.DecodeInto(b, &blk); err != nil {
		return nil, err
	}

	return &blk, nil
}

func (blk *BlockHeader) Serialize() ([]byte, error) {
	return cbor.DumpObject(blk)
}
