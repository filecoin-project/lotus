package types

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	prooftypes "github.com/filecoin-project/go-state-types/proof"
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
		ParentBaseFee:         NewInt(3432432843291),
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

func TestInteropBH(t *testing.T) {
	newAddr, err := address.NewSecp256k1Address([]byte("address0"))

	if err != nil {
		t.Fatal(err)
	}

	mcid, err := cid.Parse("bafy2bzaceaxyj7xq27gc2747adjcirpxx52tt7owqx6z6kckun7tqivvoym4y")
	if err != nil {
		t.Fatal(err)
	}

	posts := []prooftypes.PoStProof{
		{PoStProof: abi.RegisteredPoStProof_StackedDrgWinning2KiBV1, ProofBytes: []byte{0x07}},
	}

	bh := &BlockHeader{
		Miner:         newAddr,
		Ticket:        &Ticket{[]byte{0x01, 0x02, 0x03}},
		ElectionProof: &ElectionProof{0, []byte{0x0a, 0x0b}},
		BeaconEntries: []BeaconEntry{
			{
				Round: 5,
				Data:  []byte{0x0c},
				//prevRound: 0,
			},
		},
		Height:                2,
		Messages:              mcid,
		ParentMessageReceipts: mcid,
		Parents:               []cid.Cid{mcid},
		ParentWeight:          NewInt(1000),
		ForkSignaling:         3,
		ParentStateRoot:       mcid,
		Timestamp:             1,
		WinPoStProof:          posts,
		BlockSig: &crypto.Signature{
			Type: crypto.SigTypeBLS,
			Data: []byte{0x3},
		},
		BLSAggregate:  &crypto.Signature{},
		ParentBaseFee: NewInt(1000000000),
	}

	bhsb, err := bh.SigningBytes()

	if err != nil {
		t.Fatal(err)
	}

	gfc := "905501d04cb15021bf6bd003073d79e2238d4e61f1ad2281430102038200420a0b818205410c818200410781d82a5827000171a0e402202f84fef0d7cc2d7f9f00d22445f7bf7539fdd685fd9f284aa37f3822b57619cc430003e802d82a5827000171a0e402202f84fef0d7cc2d7f9f00d22445f7bf7539fdd685fd9f284aa37f3822b57619ccd82a5827000171a0e402202f84fef0d7cc2d7f9f00d22445f7bf7539fdd685fd9f284aa37f3822b57619ccd82a5827000171a0e402202f84fef0d7cc2d7f9f00d22445f7bf7539fdd685fd9f284aa37f3822b57619cc410001f60345003b9aca00"
	require.Equal(t, gfc, hex.EncodeToString(bhsb))
}

func BenchmarkBlockHeaderMarshal(b *testing.B) {
	bh := testBlockHeader(b)

	b.ReportAllocs()

	buf := new(bytes.Buffer)
	for b.Loop() {
		buf.Reset()
		if err := bh.MarshalCBOR(buf); err != nil {
			b.Fatal(err)
		}
	}
}
