package types

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	cid "github.com/ipfs/go-cid"
)

func testBlockHeader(t testing.TB) *BlockHeader {
	t.Helper()

	addr, err := address.NewIDAddress(12512063)
	if err != nil {
		t.Fatal(err)
	}

	c, err := cid.Decode("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")
	if err != nil {
		t.Fatal(err)
	}

	return &BlockHeader{
		Miner: addr,
		Ticket: &Ticket{
			VRFProof: []byte("vrf proof0000000vrf proof0000000"),
		},
		ElectionProof: &ElectionProof{
			VRFProof: []byte("vrf proof0000000vrf proof0000000"),
		},
		Parents:               []cid.Cid{c, c},
		ParentMessageReceipts: c,
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte("boo! im a signature")},
		ParentWeight:          NewInt(123125126212),
		Messages:              c,
		Height:                85919298723,
		ParentStateRoot:       c,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte("boo! im a signature")},
	}
}

func TestBlockHeaderSerialization(t *testing.T) {
	bh := testBlockHeader(t)

	buf := new(bytes.Buffer)
	if err := bh.MarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	var out BlockHeader
	if err := out.UnmarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(&out, bh) {
		fmt.Printf("%#v\n", &out)
		fmt.Printf("%#v\n", bh)
		t.Fatal("not equal")
	}
}

func BenchmarkBlockHeaderMarshal(b *testing.B) {
	bh := testBlockHeader(b)

	b.ReportAllocs()

	buf := new(bytes.Buffer)
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := bh.MarshalCBOR(buf); err != nil {
			b.Fatal(err)
		}
	}
}
