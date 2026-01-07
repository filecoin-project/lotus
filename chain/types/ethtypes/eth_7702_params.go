package ethtypes

import (
	"bytes"

	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/big"
)

// CborEncodeEIP7702Authorizations encodes the authorizationList into CBOR
// compatible with the EthAccount.ApplyAndCall / historical Delegator.ApplyDelegations params.
// Shape: a wrapper tuple with a single field `list`, where `list` is an
// array of 6-tuples [chain_id, address(20b), nonce, y_parity, r, s].
// I.e., top-level is an array with one element (the inner list).
// Intended for params to the EthAccount actor's ApplyAndCall method (and matches the older Delegator.ApplyDelegations shape).
func CborEncodeEIP7702Authorizations(list []EthAuthorization) ([]byte, error) {
	var buf bytes.Buffer
	// Write wrapper array with 1 element (the list)
	if err := cbg.CborWriteHeader(&buf, cbg.MajArray, 1); err != nil {
		return nil, err
	}
	// Write inner list header
	if err := cbg.CborWriteHeader(&buf, cbg.MajArray, uint64(len(list))); err != nil {
		return nil, err
	}
	for _, a := range list {
		if err := cbg.CborWriteHeader(&buf, cbg.MajArray, 6); err != nil {
			return nil, err
		}
		if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.ChainID)); err != nil {
			return nil, err
		}
		if err := cbg.WriteByteArray(&buf, a.Address[:]); err != nil {
			return nil, err
		}
		if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.Nonce)); err != nil {
			return nil, err
		}
		if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.YParity)); err != nil {
			return nil, err
		}
		rbig := (big.Int)(a.R)
		rb, err := rbig.Bytes()
		if err != nil {
			return nil, err
		}
		if err := cbg.WriteByteArray(&buf, rb); err != nil {
			return nil, err
		}
		sbig := (big.Int)(a.S)
		sb, err := sbig.Bytes()
		if err != nil {
			return nil, err
		}
		if err := cbg.WriteByteArray(&buf, sb); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// CborEncodeEIP7702ApplyAndCall encodes atomic apply+call params as a CBOR
// array with two elements:
//
//	[ [ tuple, ... ], [ to(20b), value(big), input(bytes) ] ]
//
// The first element is the inner list of 6-tuples (wrapper form).
// The second element carries the outer call information.
func CborEncodeEIP7702ApplyAndCall(list []EthAuthorization, to *EthAddress, value big.Int, input []byte) ([]byte, error) {
	var buf bytes.Buffer
	// Top-level array with 2 elements
	if err := cbg.CborWriteHeader(&buf, cbg.MajArray, 2); err != nil {
		return nil, err
	}
	// Element 0: inner list of authorization tuples
	if err := cbg.CborWriteHeader(&buf, cbg.MajArray, uint64(len(list))); err != nil {
		return nil, err
	}
	for _, a := range list {
		if err := cbg.CborWriteHeader(&buf, cbg.MajArray, 6); err != nil {
			return nil, err
		}
		if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.ChainID)); err != nil {
			return nil, err
		}
		if err := cbg.WriteByteArray(&buf, a.Address[:]); err != nil {
			return nil, err
		}
		if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.Nonce)); err != nil {
			return nil, err
		}
		if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.YParity)); err != nil {
			return nil, err
		}
		rbig := (big.Int)(a.R)
		rb, err := rbig.Bytes()
		if err != nil {
			return nil, err
		}
		if err := cbg.WriteByteArray(&buf, rb); err != nil {
			return nil, err
		}
		sbig := (big.Int)(a.S)
		sb, err := sbig.Bytes()
		if err != nil {
			return nil, err
		}
		if err := cbg.WriteByteArray(&buf, sb); err != nil {
			return nil, err
		}
	}
	// Element 1: call tuple [to(20b), value(big), input(bytes)]
	if err := cbg.CborWriteHeader(&buf, cbg.MajArray, 3); err != nil {
		return nil, err
	}
	// to
	var to20 [20]byte
	if to != nil {
		to20 = *to
	}
	if err := cbg.WriteByteArray(&buf, to20[:]); err != nil {
		return nil, err
	}
	// value
	vb, err := value.Bytes()
	if err != nil {
		return nil, err
	}
	if err := cbg.WriteByteArray(&buf, vb); err != nil {
		return nil, err
	}
	// input
	if err := cbg.WriteByteArray(&buf, input); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
