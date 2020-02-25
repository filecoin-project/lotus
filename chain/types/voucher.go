package types

import(
	"encoding/base64"

	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func DecodeSignedVoucher(s string) (*paych.SignedVoucher, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	var sv paych.SignedVoucher
	if err := cbor.DecodeInto(data, &sv); err != nil {
		return nil, err
	}

	return &sv, nil
}
