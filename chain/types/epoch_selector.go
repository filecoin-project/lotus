package types

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/lib/ptr"
)

// EpochSelector is a type that represents the different ways to select an epoch
// with a string that describes the epoch's relationship to the chain head.
type EpochSelector string

const (
	EpochLatest    EpochSelector = "latest"
	EpochFinalized EpochSelector = "finalized"
)

func (es *EpochSelector) UnmarshalJSON(b []byte) error {
	var selector string
	err := json.Unmarshal(b, &selector)
	if err != nil {
		return err
	}
	switch selector {
	case string(EpochLatest), string(EpochFinalized):
		*es = EpochSelector(selector)
	case "":
		*es = EpochLatest
	default:
		return fmt.Errorf(`json: invalid epoch selector ("%s")`, selector)
	}
	return nil
}

// OptionalEpochSelectorArg is a type that represents an optional epoch
// selector argument that defaults to EpochLatest. Used for zero or one argument
// jsonrpc methods that take an optional EpochSelector.
type OptionalEpochSelectorArg EpochSelector

var (
	OptionalEpochSelectorArgLatest    = json.RawMessage(`["` + EpochLatest + `"]`)
	OptionalEpochSelectorArgFinalized = json.RawMessage(`["` + EpochFinalized + `"]`)
)

func (oes *OptionalEpochSelectorArg) UnmarshalJSON(b []byte) error {
	var params []json.RawMessage
	err := json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	switch len(params) {
	case 0:
		*oes = OptionalEpochSelectorArg(EpochLatest)
	case 1:
		var selector EpochSelector
		err := json.Unmarshal(params[0], &selector)
		if err != nil {
			return err
		}
		*oes = OptionalEpochSelectorArg(selector)
	default:
		return errors.New("json: too many parameters for epoch selector")
	}
	return nil
}

// TipSetKeyOrEpochSelector is a type that represents a union of TipSetKey and
// EpochSelector. Used for jsonrpc methods that take either a TipSetKey or an
// EpochSelector.
type TipSetKeyOrEpochSelector struct {
	*TipSetKey
	*EpochSelector
}

var (
	TipSetKeyOrEpochSelectorLatest    = TipSetKeyOrEpochSelector{EpochSelector: ptr.PtrTo(EpochLatest)}
	TipSetKeyOrEpochSelectorFinalized = TipSetKeyOrEpochSelector{EpochSelector: ptr.PtrTo(EpochFinalized)}
)

func (tskes *TipSetKeyOrEpochSelector) UnmarshalJSON(b []byte) error {
	// if it starts with '[' it's a TipSetKey
	if len(b) > 0 && b[0] == '[' {
		var tsk TipSetKey
		err := json.Unmarshal(b, &tsk)
		if err != nil {
			return err
		}
		tskes.TipSetKey = &tsk
	} else {
		var selector EpochSelector
		err := json.Unmarshal(b, &selector)
		if err != nil {
			return err
		}
		tskes.EpochSelector = &selector
	}
	return nil
}

// HeightOrEpochSelector is a type that represents a union of abi.ChainEpoch and
// EpochSelector. Used for jsonrpc methods that take either a abi.ChainEpoch or
// an EpochSelector.
type HeightOrEpochSelector struct {
	*abi.ChainEpoch
	*EpochSelector
}

var (
	HeightOrEpochSelectorLatest    = HeightOrEpochSelector{EpochSelector: ptr.PtrTo(EpochLatest)}
	HeightOrEpochSelectorFinalized = HeightOrEpochSelector{EpochSelector: ptr.PtrTo(EpochFinalized)}
)

func (hes *HeightOrEpochSelector) UnmarshalJSON(b []byte) error {
	// if it starts with '"' it's an EpochSelector
	if len(b) > 0 && b[0] == '"' {
		var selector EpochSelector
		err := json.Unmarshal(b, &selector)
		if err != nil {
			return err
		}
		hes.EpochSelector = &selector
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
