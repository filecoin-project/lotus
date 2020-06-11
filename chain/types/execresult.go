package types

import "time"

type ExecutionTrace struct {
	Msg      *Message
	MsgRct   *MessageReceipt
	Error    string
	Duration time.Duration

	Subcalls []ExecutionTrace
}
