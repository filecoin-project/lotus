package chain

import (
	address "github.com/filecoin-project/go-address"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/chain/types"
)

// The created messages are retained for subsequent export or evaluation in a VM.
type MessageProducer struct {
	defaults msgOpts // Note non-pointer reference.

	messages []*types.Message
}

// NewMessageProducer creates a new message producer, delegating message creation to `factory`.
func NewMessageProducer(defaultGasLimit int64, defaultGasPrice big_spec.Int) *MessageProducer {
	return &MessageProducer{
		defaults: msgOpts{
			value:    big_spec.Zero(),
			gasLimit: defaultGasLimit,
			gasPrice: defaultGasPrice,
		},
	}
}

// Messages returns a slice containing all messages created by the producer.
func (mp *MessageProducer) Messages() []*types.Message {
	return mp.messages
}

// BuildFull creates and returns a single message.
func (mp *MessageProducer) BuildFull(from, to address.Address, method abi_spec.MethodNum, nonce uint64, value, gasPrice big_spec.Int, gasLimit int64, params []byte) *types.Message {
	fm := &types.Message{
		To:       to,
		From:     from,
		Nonce:    nonce,
		Value:    value,
		Method:   method,
		Params:   params,
		GasPrice: gasPrice,
		GasLimit: gasLimit,
	}
	mp.messages = append(mp.messages, fm)
	return fm
}

// Build creates and returns a single message, using default gas parameters unless modified by `opts`.
func (mp *MessageProducer) Build(from, to address.Address, method abi_spec.MethodNum, params []byte, opts ...MsgOpt) *types.Message {
	values := mp.defaults
	for _, opt := range opts {
		opt(&values)
	}

	return mp.BuildFull(from, to, method, values.nonce, values.value, values.gasPrice, values.gasLimit, params)
}

// msgOpts specifies value and gas parameters for a message, supporting a functional options pattern
// for concise but customizable message construction.
type msgOpts struct {
	nonce    uint64
	value    big_spec.Int
	gasPrice big_spec.Int
	gasLimit int64
}

// MsgOpt is an option configuring message value or gas parameters.
type MsgOpt func(*msgOpts)

func Value(value big_spec.Int) MsgOpt {
	return func(opts *msgOpts) {
		opts.value = value
	}
}

func Nonce(n uint64) MsgOpt {
	return func(opts *msgOpts) {
		opts.nonce = n
	}
}

func GasLimit(limit int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasLimit = limit
	}
}

func GasPrice(price int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasPrice = big_spec.NewInt(price)
	}
}
