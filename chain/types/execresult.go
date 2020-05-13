package types

type ExecutionResult struct {
	Msg    *Message
	MsgRct *MessageReceipt
	Error  string

	Subcalls []*ExecutionResult
}
