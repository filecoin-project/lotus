package chain

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/filecoin-project/go-lotus/chain/address"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/multiformats/go-multihash"
	"github.com/polydawn/refmt/obj/atlas"
)

func init() {
	ipld.Register(0x1f, IpldDecode)

	cbor.RegisterCborType(MessageReceipt{})
	cbor.RegisterCborType(Actor{})
	cbor.RegisterCborType(BlockMsg{})

	///*
	cbor.RegisterCborType(atlas.BuildEntry(BigInt{}).UseTag(2).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(i BigInt) ([]byte, error) {
				if i.Int == nil {
					return []byte{}, nil
				}

				return i.Bytes(), nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(x []byte) (BigInt, error) {
				return BigFromBytes(x), nil
			})).
		Complete())
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

				sig, err := SignatureFromBytes(sigb)
				if err != nil {
					return SignedMessage{}, err
				}

				return SignedMessage{
					Message:   x[0].(Message),
					Signature: sig,
				}, nil
			})).
		Complete())
	cbor.RegisterCborType(atlas.BuildEntry(Signature{}).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(s Signature) ([]byte, error) {
				buf := make([]byte, 4)
				n := binary.PutUvarint(buf, uint64(s.TypeCode()))
				return append(buf[:n], s.Data...), nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(x []byte) (Signature, error) {
				return SignatureFromBytes(x)
			})).
		Complete())
	cbor.RegisterCborType(atlas.BuildEntry(Message{}).UseTag(44).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(m Message) ([]interface{}, error) {
				return []interface{}{
					m.To.Bytes(),
					m.From.Bytes(),
					m.Nonce,
					m.Value,
					m.GasPrice,
					m.GasLimit,
					m.Method,
					m.Params,
				}, nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(arr []interface{}) (Message, error) {
				to, err := address.NewFromBytes(arr[0].([]byte))
				if err != nil {
					return Message{}, err
				}

				from, err := address.NewFromBytes(arr[1].([]byte))
				if err != nil {
					return Message{}, err
				}

				nonce, ok := arr[2].(uint64)
				if !ok {
					return Message{}, fmt.Errorf("expected uint64 nonce at index 2")
				}

				value := arr[3].(BigInt)
				gasPrice := arr[4].(BigInt)
				gasLimit := arr[5].(BigInt)
				method, _ := arr[6].(uint64)
				params, _ := arr[7].([]byte)

				if gasPrice.Nil() {
					gasPrice = NewInt(0)
				}

				if gasLimit.Nil() {
					gasLimit = NewInt(0)
				}

				return Message{
					To:       to,
					From:     from,
					Nonce:    nonce,
					Value:    value,
					GasPrice: gasPrice,
					GasLimit: gasLimit,
					Method:   method,
					Params:   params,
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
}

type BigInt struct {
	*big.Int
}

func NewInt(i uint64) BigInt {
	return BigInt{big.NewInt(0).SetUint64(i)}
}

func BigFromBytes(b []byte) BigInt {
	i := big.NewInt(0).SetBytes(b)
	return BigInt{i}
}

func BigMul(a, b BigInt) BigInt {
	return BigInt{big.NewInt(0).Mul(a.Int, b.Int)}
}

func BigAdd(a, b BigInt) BigInt {
	return BigInt{big.NewInt(0).Add(a.Int, b.Int)}
}

func BigSub(a, b BigInt) BigInt {
	return BigInt{big.NewInt(0).Sub(a.Int, b.Int)}
}

func BigCmp(a, b BigInt) int {
	return a.Int.Cmp(b.Int)
}

func (bi *BigInt) Nil() bool {
	return bi.Int == nil
}

func (bi *BigInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(bi.String())
}

func (bi *BigInt) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	i, ok := big.NewInt(0).SetString(s, 10)
	if !ok {
		return fmt.Errorf("failed to parse bigint string")
	}

	bi.Int = i
	return nil
}

type Actor struct {
	Code    cid.Cid
	Head    cid.Cid
	Nonce   uint64
	Balance BigInt
}

type BlockHeader struct {
	Miner address.Address

	Tickets []Ticket

	ElectionProof []byte

	Parents []cid.Cid

	ParentWeight BigInt

	Height uint64

	StateRoot cid.Cid

	Messages cid.Cid

	BLSAggregate Signature

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

type Message struct {
	To   address.Address
	From address.Address

	Nonce uint64

	Value BigInt

	GasPrice BigInt
	GasLimit BigInt

	Method uint64
	Params []byte
}

func DecodeMessage(b []byte) (*Message, error) {
	var msg Message
	if err := cbor.DecodeInto(b, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (m *Message) Serialize() ([]byte, error) {
	return cbor.DumpObject(m)
}

func (m *Message) ToStorageBlock() (block.Block, error) {
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

type MessageReceipt struct {
	ExitCode uint8

	Return []byte

	GasUsed BigInt
}

func (mr *MessageReceipt) Equals(o *MessageReceipt) bool {
	return mr.ExitCode == o.ExitCode && bytes.Equal(mr.Return, o.Return) && BigCmp(mr.GasUsed, o.GasUsed) == 0
}

type SignedMessage struct {
	Message   Message
	Signature Signature
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
	case Message:
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
