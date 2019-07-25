package chain

import (
	"encoding/json"
	"fmt"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/multiformats/go-multihash"
	"github.com/polydawn/refmt/obj/atlas"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

func init() {
	ipld.Register(0x1f, IpldDecode)

	cbor.RegisterCborType(BlockMsg{})

	///*
	//*/
	cbor.RegisterCborType(atlas.BuildEntry(SignedMessage{}).UseTag(45).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(sm SignedMessage) ([]interface{}, error) {
				return []interface{}{
					sm.Message,
					sm.Signature,
				}, nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(x []interface{}) (SignedMessage, error) {
				sigb, ok := x[1].([]byte)
				if !ok {
					return SignedMessage{}, fmt.Errorf("signature in signed message was not bytes")
				}

				sig, err := types.SignatureFromBytes(sigb)
				if err != nil {
					return SignedMessage{}, err
				}

				return SignedMessage{
					Message:   x[0].(types.Message),
					Signature: sig,
				}, nil
			})).
		Complete())
	cbor.RegisterCborType(atlas.BuildEntry(BlockHeader{}).UseTag(43).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(blk BlockHeader) ([]interface{}, error) {
				if blk.Tickets == nil {
					blk.Tickets = []Ticket{}
				}
				if blk.Parents == nil {
					blk.Parents = []cid.Cid{}
				}
				return []interface{}{
					blk.Miner.Bytes(),
					blk.Tickets,
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

				tickets := []Ticket{}
				ticketarr, _ := arr[1].([]interface{})
				for _, t := range ticketarr {
					tickets = append(tickets, Ticket(t.([]byte)))
				}
				electionProof, _ := arr[2].([]byte)

				parents := []cid.Cid{}
				parentsArr, _ := arr[3].([]interface{})
				for _, p := range parentsArr {
					parents = append(parents, p.(cid.Cid))
				}
				parentWeight := arr[4].(types.BigInt)
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
}

type BlockHeader struct {
	Miner address.Address

	Tickets []Ticket

	ElectionProof []byte

	Parents []cid.Cid

	ParentWeight types.BigInt

	Height uint64

	StateRoot cid.Cid

	Messages cid.Cid

	BLSAggregate types.Signature

	MessageReceipts cid.Cid
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

func (m *SignedMessage) ToStorageBlock() (block.Block, error) {
	data, err := m.Serialize()
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

func (m *SignedMessage) Cid() cid.Cid {
	sb, err := m.ToStorageBlock()
	if err != nil {
		panic(err)
	}

	return sb.Cid()
}

type SignedMessage struct {
	Message   types.Message
	Signature types.Signature
}

func DecodeSignedMessage(data []byte) (*SignedMessage, error) {
	var msg SignedMessage
	if err := cbor.DecodeInto(data, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (sm *SignedMessage) Serialize() ([]byte, error) {
	data, err := cbor.DumpObject(sm)
	if err != nil {
		return nil, err
	}
	return data, nil
}

type TipSet struct {
	cids   []cid.Cid
	blks   []*BlockHeader
	height uint64
}

// why didnt i just export the fields? Because the struct has methods with the
// same names already
type expTipSet struct {
	Cids   []cid.Cid
	Blocks []*BlockHeader
	Height uint64
}

func (ts *TipSet) MarshalJSON() ([]byte, error) {
	return json.Marshal(expTipSet{
		Cids:   ts.cids,
		Blocks: ts.blks,
		Height: ts.height,
	})
}

func (ts *TipSet) UnmarshalJSON(b []byte) error {
	var ets expTipSet
	if err := json.Unmarshal(b, &ets); err != nil {
		return err
	}

	ts.cids = ets.Cids
	ts.blks = ets.Blocks
	ts.height = ets.Height
	return nil
}

func NewTipSet(blks []*BlockHeader) (*TipSet, error) {
	var ts TipSet
	ts.cids = []cid.Cid{blks[0].Cid()}
	ts.blks = blks
	for _, b := range blks[1:] {
		if b.Height != blks[0].Height {
			return nil, fmt.Errorf("cannot create tipset with mismatching heights")
		}
		ts.cids = append(ts.cids, b.Cid())
	}
	ts.height = blks[0].Height

	return &ts, nil
}

func (ts *TipSet) Cids() []cid.Cid {
	return ts.cids
}

func (ts *TipSet) Height() uint64 {
	return ts.height
}

func (ts *TipSet) Weight() uint64 {
	panic("if tipsets are going to have weight on them, we need to wire that through")
}

func (ts *TipSet) Parents() []cid.Cid {
	return ts.blks[0].Parents
}

func (ts *TipSet) Blocks() []*BlockHeader {
	return ts.blks
}

func (ts *TipSet) Equals(ots *TipSet) bool {
	if len(ts.blks) != len(ots.blks) {
		return false
	}

	for i, b := range ts.blks {
		if b.Cid() != ots.blks[i].Cid() {
			return false
		}
	}

	return true
}

type Ticket []byte
type ElectionProof []byte

func IpldDecode(block block.Block) (ipld.Node, error) {
	var i interface{}
	if err := cbor.DecodeInto(block.RawData(), &i); err != nil {
		panic(err)
	}

	fmt.Println("IPLD DECODE!")
	return &filecoinIpldNode{i}, nil
}

type filecoinIpldNode struct {
	val interface{}
}

func (f *filecoinIpldNode) Cid() cid.Cid {
	switch t := f.val.(type) {
	case BlockHeader:
		return t.Cid()
	case SignedMessage:
		return t.Cid()
	default:
		panic("whats going on")
	}
}

func (f *filecoinIpldNode) Copy() ipld.Node {
	panic("no")
}

func (f *filecoinIpldNode) Links() []*ipld.Link {
	switch t := f.val.(type) {
	case BlockHeader:
		fmt.Println("block links!", t.StateRoot)
		return []*ipld.Link{
			{
				Cid: t.StateRoot,
			},
			{
				Cid: t.Messages,
			},
			{
				Cid: t.MessageReceipts,
			},
		}
	case types.Message:
		return nil
	default:
		panic("whats going on")
	}

}

func (f *filecoinIpldNode) Resolve(path []string) (interface{}, []string, error) {
	/*
		switch t := f.val.(type) {
		case Block:
			switch path[0] {
			}
		case Message:
		default:
			panic("whats going on")
		}
	*/
	panic("please dont call this")
}

// Tree lists all paths within the object under 'path', and up to the given depth.
// To list the entire object (similar to `find .`) pass "" and -1
func (f *filecoinIpldNode) Tree(path string, depth int) []string {
	panic("dont call this either")
}

func (f *filecoinIpldNode) ResolveLink(path []string) (*ipld.Link, []string, error) {
	panic("please no")
}

func (f *filecoinIpldNode) Stat() (*ipld.NodeStat, error) {
	panic("dont call this")

}

func (f *filecoinIpldNode) Size() (uint64, error) {
	panic("dont call this")
}

func (f *filecoinIpldNode) Loggable() map[string]interface{} {
	return nil
}

func (f *filecoinIpldNode) RawData() []byte {
	switch t := f.val.(type) {
	case BlockHeader:
		sb, err := t.ToStorageBlock()
		if err != nil {
			panic(err)
		}
		return sb.RawData()
	case SignedMessage:
		sb, err := t.ToStorageBlock()
		if err != nil {
			panic(err)
		}
		return sb.RawData()
	default:
		panic("whats going on")
	}
}

func (f *filecoinIpldNode) String() string {
	return "cats"
}

type FullBlock struct {
	Header   *BlockHeader
	Messages []*SignedMessage
}

func (fb *FullBlock) Cid() cid.Cid {
	return fb.Header.Cid()
}

type BlockMsg struct {
	Header   *BlockHeader
	Messages []cid.Cid
}

func DecodeBlockMsg(b []byte) (*BlockMsg, error) {
	var bm BlockMsg
	if err := cbor.DecodeInto(b, &bm); err != nil {
		return nil, err
	}

	return &bm, nil
}

func (bm *BlockMsg) Cid() cid.Cid {
	return bm.Header.Cid()
}

func (bm *BlockMsg) Serialize() ([]byte, error) {
	return cbor.DumpObject(bm)
}
