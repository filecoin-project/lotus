package rpcli

import (
	"time"
)

var (
	defaultDialTimeout = 10 * time.Second

	defaultCallOption = CallOption{
		failfast:          false,
		keepBufferedItems: false,
	}

	defaultClientOption = ClientOption{
		call:        defaultCallOption,
		dialTimeout: defaultDialTimeout,
	}
)

// CallOptionModifier modifies given call option
type CallOptionModifier = func(*CallOption)

// CallOption is the option for each rpc call
type CallOption struct {
	failfast bool

	// TODO: keep sending buffered items util any error occurs
	keepBufferedItems bool
}

// FailFast set failfast option to given value
func FailFast(b bool) CallOptionModifier {
	return func(opt *CallOption) {
		opt.failfast = b
	}
}

// ClientOptionModifier modifies given call option
type ClientOptionModifier = func(*ClientOption)

// ClientOption is the option for client
type ClientOption struct {
	call CallOption

	dialTimeout time.Duration
}

// UpdateCallOption updates call option with given modifiers
func (co *ClientOption) UpdateCallOption(opts ...CallOptionModifier) {
	for _, mod := range opts {
		mod(&co.call)
	}
}

// DialTimeout set dial timeout to given duraiton
func DialTimeout(d time.Duration) ClientOptionModifier {
	return func(opt *ClientOption) {
		opt.dialTimeout = d
	}
}
