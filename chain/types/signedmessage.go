package types

import (
	"fmt"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
	"github.com/polydawn/refmt/obj/atlas"
)

func init() {
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
	if m.Signature.Type == KTBLS {
		return m.Message.Cid()
	}

	sb, err := m.ToStorageBlock()
	if err != nil {
		panic(err)
	}

	return sb.Cid()
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
