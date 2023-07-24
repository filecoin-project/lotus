package full

import (
	"context"
	"encoding/hex"
	"fmt"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

type EthTraceAPI interface {
	TraceBlock(ctx context.Context, blkNum string) (interface{}, error)
	TraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) (interface{}, error)
}

var (
	_ EthTraceAPI = *new(api.FullNode)
)

type EthTrace struct {
	fx.In

	Chain        *store.ChainStore
	StateManager *stmgr.StateManager

	ChainAPI
	EthModuleAPI
}

var _ EthTraceAPI = (*EthTrace)(nil)

type Trace struct {
	Action       Action `json:"action"`
	Result       Result `json:"result"`
	Subtraces    int    `json:"subtraces"`
	TraceAddress []int  `json:"traceAddress"`
	Type         string `json:"Type"`
}

type TraceBlock struct {
	*Trace
	BlockHash           ethtypes.EthHash `json:"blockHash"`
	BlockNumber         int64            `json:"blockNumber"`
	TransactionHash     ethtypes.EthHash `json:"transactionHash"`
	TransactionPosition int              `json:"transactionPosition"`
}

type TraceReplayBlockTransaction struct {
	Output          string           `json:"output"`
	StateDiff       *string          `json:"stateDiff"`
	Trace           []*Trace         `json:"trace"`
	TransactionHash ethtypes.EthHash `json:"transactionHash"`
	VmTrace         *string          `json:"vmTrace"`
}

type Action struct {
	CallType string             `json:"callType"`
	From     string             `json:"from"`
	To       string             `json:"to"`
	Gas      ethtypes.EthUint64 `json:"gas"`
	Input    string             `json:"input"`
	Value    ethtypes.EthBigInt `json:"value"`
}

type Result struct {
	GasUsed ethtypes.EthUint64 `json:"gasUsed"`
	Output  string             `json:"output"`
}

func (e *EthTrace) TraceBlock(ctx context.Context, blkNum string) (interface{}, error) {
	ts, err := e.getTipsetByBlockNr(ctx, blkNum, false)
	if err != nil {
		return nil, err
	}

	_, trace, err := e.StateManager.ExecutionTrace(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute base state: %w", err)
	}

	tsParent, err := e.ChainAPI.ChainGetTipSetByHeight(ctx, ts.Height()+1, e.Chain.GetHeaviestTipSet().Key())
	if err != nil {
		return nil, fmt.Errorf("cannot get tipset at height: %v", ts.Height()+1)
	}

	msgs, err := e.ChainGetParentMessages(ctx, tsParent.Blocks()[0].Cid())
	if err != nil {
		return nil, err
	}

	cid, err := ts.Key().Cid()
	if err != nil {
		return nil, err
	}

	blkHash, err := ethtypes.EthHashFromCid(cid)
	if err != nil {
		return nil, err
	}

	allTraces := make([]*TraceBlock, 0, len(trace))
	for _, ir := range trace {
		// ignore messages from f00
		if ir.Msg.From.String() == "f00" {
			continue
		}

		idx := -1
		for msgIdx, msg := range msgs {
			if ir.Msg.From == msg.Message.From {
				idx = msgIdx
				break
			}
		}
		if idx == -1 {
			log.Warnf("cannot resolve message index for cid: %s", ir.MsgCid)
			continue
		}

		txHash, err := e.EthGetTransactionHashByCid(ctx, ir.MsgCid)
		if err != nil {
			return nil, err
		}
		if txHash == nil {
			log.Warnf("cannot find transaction hash for cid %s", ir.MsgCid)
			continue
		}

		traces := []*Trace{}
		buildTraces(&traces, []int{}, ir.ExecutionTrace)

		traceBlocks := make([]*TraceBlock, 0, len(trace))
		for _, trace := range traces {
			traceBlocks = append(traceBlocks, &TraceBlock{
				Trace:               trace,
				BlockHash:           blkHash,
				BlockNumber:         int64(ts.Height()),
				TransactionHash:     *txHash,
				TransactionPosition: idx,
			})
		}

		allTraces = append(allTraces, traceBlocks...)
	}

	return allTraces, nil
}

func (e *EthTrace) TraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) (interface{}, error) {
	if len(traceTypes) != 1 || traceTypes[0] != "trace" {
		return nil, fmt.Errorf("only 'trace' is supported")
	}

	ts, err := e.getTipsetByBlockNr(ctx, blkNum, false)
	if err != nil {
		return nil, err
	}

	_, trace, err := e.StateManager.ExecutionTrace(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed when calling ExecutionTrace: %w", err)
	}

	allTraces := make([]*TraceReplayBlockTransaction, 0, len(trace))
	for _, ir := range trace {
		// ignore messages from f00
		if ir.Msg.From.String() == "f00" {
			continue
		}

		txHash, err := e.EthGetTransactionHashByCid(ctx, ir.MsgCid)
		if err != nil {
			return nil, err
		}
		if txHash == nil {
			log.Warnf("cannot find transaction hash for cid %s", ir.MsgCid)
			continue
		}

		t := TraceReplayBlockTransaction{
			Output:          hex.EncodeToString(ir.MsgRct.Return),
			TransactionHash: *txHash,
			StateDiff:       nil,
			VmTrace:         nil,
		}

		buildTraces(&t.Trace, []int{}, ir.ExecutionTrace)

		allTraces = append(allTraces, &t)
	}

	return allTraces, nil
}

// buildTraces recursively builds the traces for a given ExecutionTrace by walking the subcalls
func buildTraces(traces *[]*Trace, addr []int, et types.ExecutionTrace) {
	callType := "call"
	if et.Msg.ReadOnly {
		callType = "staticcall"
	}

	// TODO: add check for determining if this this should be delegatecall
	if false {
		callType = "delegatecall"
	}

	*traces = append(*traces, &Trace{
		Action: Action{
			CallType: callType,
			From:     et.Msg.From.String(),
			To:       et.Msg.To.String(),
			Gas:      ethtypes.EthUint64(et.Msg.GasLimit),
			Input:    hex.EncodeToString(et.Msg.Params),
			Value:    ethtypes.EthBigInt(et.Msg.Value),
		},
		Result: Result{
			GasUsed: ethtypes.EthUint64(et.SumGas().TotalGas),
			Output:  hex.EncodeToString(et.MsgRct.Return),
		},
		Subtraces:    len(et.Subcalls),
		TraceAddress: addr,
		Type:         callType,
	})

	for i, call := range et.Subcalls {
		buildTraces(traces, append(addr, i), call)
	}
}

// TODO: refactor this to be shared code
func (e *EthTrace) getTipsetByBlockNr(ctx context.Context, blkParam string, strict bool) (*types.TipSet, error) {
	if blkParam == "earliest" {
		return nil, fmt.Errorf("block param \"earliest\" is not supported")
	}

	head := e.Chain.GetHeaviestTipSet()
	switch blkParam {
	case "pending":
		return head, nil
	case "latest":
		parent, err := e.Chain.GetTipSetFromKey(ctx, head.Parents())
		if err != nil {
			return nil, fmt.Errorf("cannot get parent tipset")
		}
		return parent, nil
	default:
		var num ethtypes.EthUint64
		err := num.UnmarshalJSON([]byte(`"` + blkParam + `"`))
		if err != nil {
			return nil, fmt.Errorf("cannot parse block number: %v", err)
		}
		if abi.ChainEpoch(num) > head.Height()-1 {
			return nil, fmt.Errorf("requested a future epoch (beyond 'latest')")
		}
		ts, err := e.ChainAPI.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(num), head.Key())
		if err != nil {
			return nil, fmt.Errorf("cannot get tipset at height: %v", num)
		}
		if strict && ts.Height() != abi.ChainEpoch(num) {
			return nil, ErrNullRound
		}
		return ts, nil
	}
}
