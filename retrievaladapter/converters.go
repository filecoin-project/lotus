package retrievaladapter

import (
	"bytes"
	sharedamount "github.com/filecoin-project/go-fil-components/shared/tokenamount"
	sharedtypes "github.com/filecoin-project/go-fil-components/shared/types"
	"github.com/filecoin-project/lotus/chain/types"
)


func FromSharedTokenAmount(in sharedamount.TokenAmount) types.BigInt {
	return types.BigInt{Int: in.Int}
}

func ToSharedTokenAmount(in types.BigInt) sharedamount.TokenAmount {
	return sharedamount.TokenAmount{Int: in.Int}
}

func ToSharedSignedVoucher(in *types.SignedVoucher) (*sharedtypes.SignedVoucher, error) {
	var encoded bytes.Buffer
	err := in.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out sharedtypes.SignedVoucher
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func FromSharedSignedVoucher(in *sharedtypes.SignedVoucher) (*types.SignedVoucher, error) {
	var encoded bytes.Buffer
	err := in.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out types.SignedVoucher
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
