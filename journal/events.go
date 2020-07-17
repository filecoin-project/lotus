package journal

import (
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

type HeadChangeEvt struct {
	From        types.TipSetKey `json:"from"`
	FromHeight  abi.ChainEpoch  `json:"from_height"`
	To          types.TipSetKey `json:"to"`
	ToHeight    abi.ChainEpoch  `json:"to_height"`
	RevertCount int             `json:"rev_cnt"`
	ApplyCount  int             `json:"apply_cnt"`
}
