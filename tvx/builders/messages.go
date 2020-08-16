package builders

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
)

// TypedCall represents a call to a known built-in actor kind.
type TypedCall func() (method abi.MethodNum, params []byte)

// Messages accumulates the messages to be executed within the test vector.
type Messages struct {
	b        *Builder
	defaults msgOpts

	messages []*ApplicableMessage
}

// SetDefaults sets default options for all messages.
func (m *Messages) SetDefaults(opts ...MsgOpt) *Messages {
	for _, opt := range opts {
		opt(&m.defaults)
	}
	return m
}

// ApplicableMessage represents a message to be applied on the test vector.
type ApplicableMessage struct {
	Epoch   abi.ChainEpoch
	Message *types.Message
	Result  *vm.ApplyRet
}

func (m *Messages) Sugar() *sugarMsg {
	return &sugarMsg{m}
}

// All returns all ApplicableMessages that have been accumulated, in the same
// order they were added.
func (m *Messages) All() []*ApplicableMessage {
	cpy := make([]*ApplicableMessage, len(m.messages))
	copy(cpy, m.messages)
	return cpy
}

// Typed adds a typed call to this message accumulator.
func (m *Messages) Typed(from, to address.Address, typedm TypedCall, opts ...MsgOpt) *ApplicableMessage {
	method, params := typedm()
	return m.Raw(from, to, method, params, opts...)
}

// Raw adds a raw message to this message accumulator.
func (m *Messages) Raw(from, to address.Address, method abi.MethodNum, params []byte, opts ...MsgOpt) *ApplicableMessage {
	options := m.defaults
	for _, opt := range opts {
		opt(&options)
	}

	msg := &types.Message{
		To:         to,
		From:       from,
		Nonce:      options.nonce,
		Value:      options.value,
		Method:     method,
		Params:     params,
		GasLimit:   options.gasLimit,
		GasFeeCap:  options.gasFeeCap,
		GasPremium: options.gasPremium,
	}

	am := &ApplicableMessage{
		Epoch:   options.epoch,
		Message: msg,
	}

	m.messages = append(m.messages, am)
	return am
}

// ApplyOne applies the provided message. The following constraints are checked:
//  - all previous messages have been applied.
//  - we know about this message (i.e. it has been added through Typed, Raw or Sugar).
func (m *Messages) ApplyOne(am *ApplicableMessage) {
	var found bool
	for i, other := range m.messages {
		if other.Result != nil {
			// message has been applied, continue.
			continue
		}
		if am == other {
			// we have scanned all preceding messages, and verified they had been applied.
			// we are ready to perform the application.
			found = true
			break
		}
		// verify that preceding messages have been applied.
		// this will abort if unsatisfied.
		m.b.Assert.Nil(other.Result, "preceding messages must have been applied when calling Apply*; index of first unapplied: %d", i)
	}
	m.b.Assert.True(found, "ApplicableMessage not found")
	m.b.applyMessage(am)
}

// ApplyN calls ApplyOne for the supplied messages, in the order they are passed.
// The constraints described in ApplyOne apply.
func (m *Messages) ApplyN(ams ...*ApplicableMessage) {
	for _, am := range ams {
		m.ApplyOne(am)
	}
}

type msgOpts struct {
	nonce      uint64
	value      big.Int
	gasLimit   int64
	gasFeeCap  abi.TokenAmount
	gasPremium abi.TokenAmount
	epoch      abi.ChainEpoch
}

// MsgOpt is an option configuring message value, gas parameters, execution
// epoch, and other elements.
type MsgOpt func(*msgOpts)

// Value sets a value on a message.
func Value(value big.Int) MsgOpt {
	return func(opts *msgOpts) {
		opts.value = value
	}
}

// Nonce sets the nonce of a message.
func Nonce(n uint64) MsgOpt {
	return func(opts *msgOpts) {
		opts.nonce = n
	}
}

// GasLimit sets the gas limit of a message.
func GasLimit(limit int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasLimit = limit
	}
}

// GasFeeCap sets the gas fee cap of a message.
func GasFeeCap(feeCap int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasFeeCap = big.NewInt(feeCap)
	}
}

// GasPremium sets the gas premium of a message.
func GasPremium(premium int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasPremium = big.NewInt(premium)
	}
}

// Epoch sets the epoch in which a message is to be executed.
func Epoch(epoch abi.ChainEpoch) MsgOpt {
	return func(opts *msgOpts) {
		opts.epoch = epoch
	}
}
