package stmgr

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

type ExecMonitor interface {
	// MessageApplied is called after a message has been applied. Returning an error will halt execution of any further messages.
	MessageApplied(ctx context.Context, ts *types.TipSet, mcid cid.Cid, msg *types.Message, ret *vm.ApplyRet, implicit bool) error
}

var _ ExecMonitor = (*InvocationTracer)(nil)

type InvocationTracer struct {
	trace *[]*api.InvocResult
}

func (i *InvocationTracer) MessageApplied(ctx context.Context, ts *types.TipSet, mcid cid.Cid, msg *types.Message, ret *vm.ApplyRet, implicit bool) error {
	ir := &api.InvocResult{
		MsgCid:         mcid,
		Msg:            msg,
		MsgRct:         &ret.MessageReceipt,
		ExecutionTrace: ret.ExecutionTrace,
		Duration:       ret.Duration,
	}
	if ret.ActorErr != nil {
		ir.Error = ret.ActorErr.Error()
	}
	if ret.GasCosts != nil {
		ir.GasCost = MakeMsgGasCost(msg, ret)
	}
	*i.trace = append(*i.trace, ir)
	return nil
}

var _ ExecMonitor = (*messageFinder)(nil)

type messageFinder struct {
	mcid cid.Cid // the message cid to find
	outm *types.Message
	outr *vm.ApplyRet
}

func (m *messageFinder) MessageApplied(ctx context.Context, ts *types.TipSet, mcid cid.Cid, msg *types.Message, ret *vm.ApplyRet, implicit bool) error {
	if m.mcid == mcid {
		m.outm = msg
		m.outr = ret
		return errHaltExecution // message was found, no need to continue
	}
	return nil
}
