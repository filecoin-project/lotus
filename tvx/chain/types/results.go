package types

import (
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
)

type Trackable interface {
	GoSyntax() string
	GoContainer() string
}

var _ Trackable = (*ApplyMessageResult)(nil)
var _ Trackable = (*ApplyTipSetResult)(nil)

type ApplyMessageResult struct {
	Receipt MessageReceipt
	Penalty abi.TokenAmount
	Reward  abi.TokenAmount
	Root    string
}

func (mr ApplyMessageResult) GoSyntax() string {
	return fmt.Sprintf("types.ApplyMessageResult{Receipt: %#v, Penalty: abi.NewTokenAmount(%d), Reward: abi.NewTokenAmount(%d), Root: \"%s\"}", mr.Receipt, mr.Penalty, mr.Reward, mr.Root)
}

func (mr ApplyMessageResult) GoContainer() string {
	return "[]types.ApplyMessageResult"
}

func (mr ApplyMessageResult) StateRoot() cid.Cid {
	root, err := cid.Decode(mr.Root)
	if err != nil {
		panic(err)
	}
	return root
}

func (mr ApplyMessageResult) GasUsed() GasUnits {
	return mr.Receipt.GasUsed
}

type ApplyTipSetResult struct {
	Receipts []MessageReceipt
	Root     string
}

func (tr ApplyTipSetResult) GoSyntax() string {
	return fmt.Sprintf("%#v", tr)
}

func (tr ApplyTipSetResult) GoContainer() string {
	return "[]types.ApplyTipSetResult"
}

func (tr ApplyTipSetResult) StateRoot() cid.Cid {
	root, err := cid.Decode(tr.Root)
	if err != nil {
		panic(err)
	}
	return root
}
