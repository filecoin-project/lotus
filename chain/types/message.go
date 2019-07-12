package types

import (
	"fmt"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
	"github.com/polydawn/refmt/obj/atlas"

	"github.com/filecoin-project/go-lotus/chain/address"
)

func init() {
	cbor.RegisterCborType(atlas.BuildEntry(Message{}).UseTag(44).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(m Message) ([]interface{}, error) {
				return []interface{}{
					m.To.Bytes(),
					m.From.Bytes(),
					m.Nonce,
					m.Value,
					m.GasPrice,
					m.GasLimit,
					m.Method,
					m.Params,
				}, nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(arr []interface{}) (Message, error) {
				to, err := address.NewFromBytes(arr[0].([]byte))
				if err != nil {
					return Message{}, err
				}

				from, err := address.NewFromBytes(arr[1].([]byte))
				if err != nil {
					return Message{}, err
				}

				nonce, ok := arr[2].(uint64)
				if !ok {
					return Message{}, fmt.Errorf("expected uint64 nonce at index 2")
				}

				value := arr[3].(BigInt)
				gasPrice := arr[4].(BigInt)
				gasLimit := arr[5].(BigInt)
				method, _ := arr[6].(uint64)
				params, _ := arr[7].([]byte)

				if gasPrice.Nil() {
					gasPrice = NewInt(0)
				}

				if gasLimit.Nil() {
					gasLimit = NewInt(0)
				}

				return Message{
					To:       to,
					From:     from,
					Nonce:    nonce,
					Value:    value,
					GasPrice: gasPrice,
					GasLimit: gasLimit,
					Method:   method,
					Params:   params,
				}, nil
			})).
		Complete())
}

type Message struct {
	To   address.Address
	From address.Address

	Nonce uint64

	Value BigInt

	GasPrice BigInt
	GasLimit BigInt

	Method uint64
	Params []byte
}

func DecodeMessage(b []byte) (*Message, error) {
	var msg Message
	if err := cbor.DecodeInto(b, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (m *Message) Serialize() ([]byte, error) {
	return cbor.DumpObject(m)
}

func (m *Message) ToStorageBlock() (block.Block, error) {
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
