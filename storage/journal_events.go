package storage

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/ipfs/go-cid"
)

type WindowPoStEvt struct {
	State    string
	Deadline *miner.DeadlineInfo
	Height   abi.ChainEpoch
	TipSet   []cid.Cid
	Error    error `json:",omitempty"`

	Proofs     *WindowPoStEvt_Proofs     `json:",omitempty"`
	Recoveries *WindowPoStEvt_Recoveries `json:",omitempty"`
	Faults     *WindowPoStEvt_Faults     `json:",omitempty"`
}

type WindowPoStEvt_Proofs struct {
	Partitions []miner.PoStPartition
	MessageCID cid.Cid
}

type WindowPoStEvt_Recoveries struct {
	Declarations []miner.RecoveryDeclaration
	MessageCID   cid.Cid
}

type WindowPoStEvt_Faults struct {
	Declarations []miner.FaultDeclaration
	MessageCID   cid.Cid
}
