package types

import (
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/lib/ptr"
)

// EpochDescriptor is a type that represents the different ways to select an
// epoch with a string that describes the epoch's relationship to the chain
// head.
type EpochDescriptor string

const (
	EpochLatest    EpochDescriptor = "latest"
	EpochFinalized EpochDescriptor = "finalized"
)

func (es *EpochDescriptor) UnmarshalJSON(b []byte) error {
	var selector string
	err := json.Unmarshal(b, &selector)
	if err != nil {
		return err
	}
	switch selector {
	case string(EpochLatest), string(EpochFinalized):
		*es = EpochDescriptor(selector)
	case "":
		*es = EpochLatest
	default:
		return fmt.Errorf(`json: invalid epoch selector ("%s")`, selector)
	}
	return nil
}

// OptionalEpochDescriptorArg is a type that represents an optional epoch
// selector argument that defaults to EpochLatest. Used for zero or one argument
// jsonrpc methods that take an optional EpochDescriptor.
type OptionalEpochDescriptorArg EpochDescriptor

var (
	OptionalEpochDescriptorArgLatest    = json.RawMessage(`["` + EpochLatest + `"]`)
	OptionalEpochDescriptorArgFinalized = json.RawMessage(`["` + EpochFinalized + `"]`)
)

func DecodeOptionalEpochDescriptorArg(p jsonrpc.RawParams) (EpochDescriptor, error) {
	if p == nil {
		return EpochLatest, nil
	}
	if param, err := jsonrpc.DecodeParams[OptionalEpochDescriptorArg](p); err != nil {
		return "", xerrors.Errorf("json decoding: %w", err)
	} else {
		return EpochDescriptor(param), nil
	}
}

func (oes *OptionalEpochDescriptorArg) UnmarshalJSON(b []byte) error {
	var params []json.RawMessage
	if b != nil {
		if err := json.Unmarshal(b, &params); err != nil {
			return err
		}
	}

	switch len(params) {
	case 0:
		*oes = OptionalEpochDescriptorArg(EpochLatest)
	case 1:
		var selector EpochDescriptor
		err := json.Unmarshal(params[0], &selector)
		if err != nil {
			return err
		}
		*oes = OptionalEpochDescriptorArg(selector)
	default:
		return errors.New("json: too many parameters for epoch selector")
	}
	return nil
}

// TipSetSelector is a type that represents a union of TipSetKey and
// EpochDescriptor. Used for API methods that take either a TipSetKey or an
// EpochDescriptor.
type TipSetSelector struct {
	*TipSetKey
	*EpochDescriptor
}

var (
	TipSetSelectorLatest    = TipSetSelector{EpochDescriptor: ptr.PtrTo(EpochLatest)}
	TipSetSelectorFinalized = TipSetSelector{EpochDescriptor: ptr.PtrTo(EpochFinalized)}
)

func NewTipSetSelector(tsk TipSetKey) TipSetSelector {
	return TipSetSelector{TipSetKey: &tsk}
}

func (tskes *TipSetSelector) UnmarshalJSON(b []byte) error {
	// if it starts with '[' it's a TipSetKey
	if len(b) > 0 && b[0] == '[' {
		var tsk TipSetKey
		err := json.Unmarshal(b, &tsk)
		if err != nil {
			return err
		}
		tskes.TipSetKey = &tsk
	} else {
		var selector EpochDescriptor
		err := json.Unmarshal(b, &selector)
		if err != nil {
			return err
		}
		tskes.EpochDescriptor = &selector
	}
	return nil
}

// EpochSelector is a type that represents a union of abi.ChainEpoch and
// EpochDescriptor. Used for jsonrpc methods that take either a abi.ChainEpoch
// or an EpochDescriptor.
type EpochSelector struct {
	*abi.ChainEpoch
	*EpochDescriptor
}

var (
	EpochSelectorLatest    = EpochSelector{EpochDescriptor: ptr.PtrTo(EpochLatest)}
	EpochSelectorFinalized = EpochSelector{EpochDescriptor: ptr.PtrTo(EpochFinalized)}
)

func NewEpochSelector(epoch abi.ChainEpoch) EpochSelector {
	return EpochSelector{ChainEpoch: ptr.PtrTo(epoch)}
}

func (hes *EpochSelector) UnmarshalJSON(b []byte) error {
	// if it starts with '"' it's an EpochDescriptor
	if len(b) > 0 && b[0] == '"' {
		var selector EpochDescriptor
		err := json.Unmarshal(b, &selector)
		if err != nil {
			return err
		}
		hes.EpochDescriptor = &selector
	} else {
		var height abi.ChainEpoch
		err := json.Unmarshal(b, &height)
		if err != nil {
			return err
		}
		hes.ChainEpoch = &height
	}
	return nil
}
