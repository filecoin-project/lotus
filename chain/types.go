package chain

import (
	"encoding/json"
	"fmt"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"

	"github.com/filecoin-project/go-lotus/chain/types"
)

func init() {
	ipld.Register(0x1f, IpldDecode)

	cbor.RegisterCborType(BlockMsg{})
}

type TipSet struct {
	cids   []cid.Cid
	blks   []*types.BlockHeader
	height uint64
}

// why didnt i just export the fields? Because the struct has methods with the
// same names already
type expTipSet struct {
	Cids   []cid.Cid
	Blocks []*types.BlockHeader
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

func NewTipSet(blks []*types.BlockHeader) (*TipSet, error) {
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

func (ts *TipSet) Blocks() []*types.BlockHeader {
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
	case types.BlockHeader:
		return t.Cid()
	case types.SignedMessage:
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
	case types.BlockHeader:
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
	case types.BlockHeader:
		sb, err := t.ToStorageBlock()
		if err != nil {
			panic(err)
		}
		return sb.RawData()
	case types.SignedMessage:
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

type BlockMsg struct {
	Header   *types.BlockHeader
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
