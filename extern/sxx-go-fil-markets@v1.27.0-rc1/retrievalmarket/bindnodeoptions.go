package retrievalmarket

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
)

// go type converter functions for bindnode for common Filecoin data types

// CborGenCompatibleNodeBindnodeOption converts a CborGenCompatibleNode type to
// and from an Any field in a schema
var CborGenCompatibleNodeBindnodeOption = bindnode.TypedAnyConverter(&CborGenCompatibleNode{}, cborGenCompatibleNodeFromAny, cborGenCompatibleNodeToAny)

// BigIntBindnodeOption converts a big.Int type to and from a Bytes field in a
// schema
var BigIntBindnodeOption = bindnode.TypedBytesConverter(&big.Int{}, bigIntFromBytes, bigIntToBytes)

// TokenAmountBindnodeOption converts a filecoin abi.TokenAmount type to and
// from a Bytes field in a schema
var TokenAmountBindnodeOption = bindnode.TypedBytesConverter(&abi.TokenAmount{}, tokenAmountFromBytes, tokenAmountToBytes)

// AddressBindnodeOption converts a filecoin Address type to and from a Bytes
// field in a schema
var AddressBindnodeOption = bindnode.TypedBytesConverter(&address.Address{}, addressFromBytes, addressToBytes)

// SignatureBindnodeOption converts a filecoin Signature type to and from a
// Bytes field in a schema
var SignatureBindnodeOption = bindnode.TypedBytesConverter(&crypto.Signature{}, signatureFromBytes, signatureToBytes)

// CborGenCompatibleNode is for cbor-gen / go-ipld-prime compatibility, to
// replace Deferred types that are used to represent datamodel.Nodes.
// This shouldn't be used as a pointer (nullable/optional) as it can consume
// "Null" tokens and therefore be a Null. Instead, use
// CborGenCompatibleNode#IsNull to check for null status.
type CborGenCompatibleNode struct {
	Node datamodel.Node
}

func (sn CborGenCompatibleNode) IsNull() bool {
	return sn.Node == nil || sn.Node == datamodel.Null
}

// UnmarshalCBOR is for cbor-gen compatibility
func (sn *CborGenCompatibleNode) UnmarshalCBOR(r io.Reader) error {
	// use cbg.Deferred.UnmarshalCBOR to figure out how much to pull
	def := cbg.Deferred{}
	if err := def.UnmarshalCBOR(r); err != nil {
		return err
	}
	// convert it to a Node
	na := basicnode.Prototype.Any.NewBuilder()
	if err := dagcbor.Decode(na, bytes.NewReader(def.Raw)); err != nil {
		return err
	}
	sn.Node = na.Build()
	return nil
}

// MarshalCBOR is for cbor-gen compatibility
func (sn *CborGenCompatibleNode) MarshalCBOR(w io.Writer) error {
	node := datamodel.Null
	if sn != nil && sn.Node != nil {
		node = sn.Node
		if tn, ok := node.(schema.TypedNode); ok {
			node = tn.Representation()
		}
	}
	return dagcbor.Encode(node, w)
}

func cborGenCompatibleNodeFromAny(node datamodel.Node) (interface{}, error) {
	return &CborGenCompatibleNode{Node: node}, nil
}

func cborGenCompatibleNodeToAny(iface interface{}) (datamodel.Node, error) {
	sn, ok := iface.(*CborGenCompatibleNode)
	if !ok {
		return nil, fmt.Errorf("expected *CborGenCompatibleNode value")
	}
	if sn.Node == nil {
		return datamodel.Null, nil
	}
	return sn.Node, nil
}

func tokenAmountFromBytes(b []byte) (interface{}, error) {
	return bigIntFromBytes(b)
}

func bigIntFromBytes(b []byte) (interface{}, error) {
	if len(b) == 0 {
		return big.NewInt(0), nil
	}
	return big.FromBytes(b)
}

func tokenAmountToBytes(iface interface{}) ([]byte, error) {
	return bigIntToBytes(iface)
}

func bigIntToBytes(iface interface{}) ([]byte, error) {
	bi, ok := iface.(*big.Int)
	if !ok {
		return nil, fmt.Errorf("expected *big.Int value")
	}
	if bi == nil || bi.Int == nil {
		*bi = big.Zero()
	}
	return bi.Bytes()
}

func addressFromBytes(b []byte) (interface{}, error) {
	return address.NewFromBytes(b)
}

func addressToBytes(iface interface{}) ([]byte, error) {
	addr, ok := iface.(*address.Address)
	if !ok {
		return nil, fmt.Errorf("expected *Address value")
	}
	return addr.Bytes(), nil
}

// Signature is a byteprefix union
func signatureFromBytes(b []byte) (interface{}, error) {
	if len(b) > crypto.SignatureMaxLength {
		return nil, fmt.Errorf("string too long")
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("string empty")
	}
	var s crypto.Signature
	switch crypto.SigType(b[0]) {
	default:
		return nil, fmt.Errorf("invalid signature type in cbor input: %d", b[0])
	case crypto.SigTypeSecp256k1:
		s.Type = crypto.SigTypeSecp256k1
	case crypto.SigTypeBLS:
		s.Type = crypto.SigTypeBLS
	}
	s.Data = b[1:]
	return &s, nil
}

func signatureToBytes(iface interface{}) ([]byte, error) {
	s, ok := iface.(*crypto.Signature)
	if !ok {
		return nil, fmt.Errorf("expected *Signature value")
	}
	ba := append([]byte{byte(s.Type)}, s.Data...)
	return ba, nil
}
