package chain

import (
	"bytes"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-lotus/chain/types"
)

func init() {
	//ipld.Register(0x1f, IpldDecode)

	cbor.RegisterCborType(BlockMsg{})
}

/*
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
		fmt.Printf("bad type: %T\n", f.val)
		fmt.Printf("what even is this: %#v\n", f.val)
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
	//
		//switch t := f.val.(type) {
		//case Block:
			//switch path[0] {
			//}
		//case Message:
		//default:
			//panic("whats going on")
		//}
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
*/

type BlockMsg struct {
	Header        *types.BlockHeader
	BlsMessages   []cid.Cid
	SecpkMessages []cid.Cid
}

func DecodeBlockMsg(b []byte) (*BlockMsg, error) {
	var bm BlockMsg
	if err := bm.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, err
	}

	return &bm, nil
}

func (bm *BlockMsg) Cid() cid.Cid {
	return bm.Header.Cid()
}

func (bm *BlockMsg) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := bm.MarshalCBOR(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
