package chain

import (
	"encoding/json"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestSignedMessageJsonRoundtrip(t *testing.T) {
	to, _ := address.NewIDAddress(5234623)
	from, _ := address.NewIDAddress(603911192)
	smsg := &types.SignedMessage{
		Message: types.Message{
			To:       to,
			From:     from,
			Params:   []byte("some bytes, idk"),
			Method:   1235126,
			Value:    types.NewInt(123123),
			GasPrice: types.NewInt(1234),
			GasLimit: types.NewInt(9992969384),
			Nonce:    123123,
		},
	}

	out, err := json.Marshal(smsg)
	if err != nil {
		t.Fatal(err)
	}

	var osmsg types.SignedMessage
	if err := json.Unmarshal(out, &osmsg); err != nil {
		t.Fatal(err)
	}
}
