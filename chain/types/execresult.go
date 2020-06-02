package types

import "time"

type ExecutionResult struct {
	Msg      *Message
	MsgRct   *MessageReceipt
	Error    string
	Duration time.Duration

	Subcalls []*ExecutionResult
}
