package types

import (
	"encoding/json"
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
)

type GasTrace struct {
	Name       string
	TotalGas   int64         `json:"tg"`
	ComputeGas int64         `json:"cg"`
	StorageGas int64         `json:"sg"`
	TimeTaken  time.Duration `json:"tt"`
}

type MessageTrace struct {
	From        address.Address
	To          address.Address
	Value       abi.TokenAmount
	Method      abi.MethodNum
	Params      []byte
	ParamsCodec uint64
	GasLimit    uint64
	ReadOnly    bool
}

type ActorTrace struct {
	Id    abi.ActorID
	State Actor
}

type ReturnTrace struct {
	ExitCode    exitcode.ExitCode
	Return      []byte
	ReturnCodec uint64
}

type Op uint64

const (
	IpldOpGet Op = iota
	IpldOpPut
)

type TraceIpld struct {
	Op   Op
	Cid  cid.Cid
	Size uint64
}

func (t TraceIpld) MarshalJSON() ([]byte, error) {
	type TraceIpldJSON struct {
		Op   string
		Cid  cid.Cid
		Size uint64
	}

	var opStr string
	switch t.Op {
	case IpldOpGet:
		opStr = "Get"
	case IpldOpPut:
		opStr = "Put"
	default:
		opStr = "Unknown"
	}

	return json.Marshal(TraceIpldJSON{
		Op:   opStr,
		Cid:  t.Cid,
		Size: t.Size,
	})
}

func (t *TraceIpld) UnmarshalJSON(data []byte) error {
	type TraceIpldJSON struct {
		Op   string
		Cid  cid.Cid
		Size uint64
	}

	var tj TraceIpldJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return err
	}

	t.Cid = tj.Cid
	t.Size = tj.Size

	switch tj.Op {
	case "Get":
		t.Op = IpldOpGet
	case "Put":
		t.Op = IpldOpPut
	default:
		return xerrors.Errorf("unknown operation: %s", tj.Op)
	}

	return nil
}

type ExecutionTrace struct {
	Msg          MessageTrace
	MsgRct       ReturnTrace
	InvokedActor *ActorTrace      `json:",omitempty"`
	GasCharges   []*GasTrace      `cborgen:"maxlen=1000000000"`
	Subcalls     []ExecutionTrace `cborgen:"maxlen=1000000000"`
	Logs         []string         `cborgen:"maxlen=1000000000" json:",omitempty"`
	IpldOps      []TraceIpld      `cborgen:"maxlen=1000000000" json:",omitempty"`
}

func (et ExecutionTrace) SumGas() GasTrace {
	return SumGas(et.GasCharges)
}

func SumGas(charges []*GasTrace) GasTrace {
	var out GasTrace
	for _, gc := range charges {
		out.TotalGas += gc.TotalGas
		out.ComputeGas += gc.ComputeGas
		out.StorageGas += gc.StorageGas
	}

	return out
}

func (gt *GasTrace) MarshalJSON() ([]byte, error) {
	type GasTraceCopy GasTrace
	cpy := (*GasTraceCopy)(gt)
	return json.Marshal(cpy)
}
