package finality_sync

import (
	types "github.com/filecoin-project/lotus/chain/types/finality_sync"
	"golang.org/x/xerrors"
)

type FinalityState struct {
	lastFinalizedEpoch        int64
	lastGraniteInstanceNumber int64
	powerTable                types.PowerTable
}

func (fs *FinalityState) ValidateFinalityCertificate(fc *types.FinalityCertificate) error {
	if fc == nil {
		return nil
	}

	// TODO(jie): Validate voter's identity and total power
	//   我这里还没有写的原因是Aayush还没有sign off on FinalityState的数据结构是不是一定长成
	//   那个样子。如果是的话，这里写的validation才有意义。否则如果对于比如说powertable的包含有不同想法，
	//   那么我这里的verification写了也白写

	// TODO(jie): Validate blssignature

	if fs.lastFinalizedEpoch >= fc.GraniteDecision.Epoch {
		return xerrors.Errorf("last finalized epoch %d >= proposed finalized epoch %d", fs.lastFinalizedEpoch, fc.GraniteDecision.Epoch)
	}
	if fs.lastGraniteInstanceNumber >= fc.GraniteDecision.InstanceNumber {
		return xerrors.Errorf("last granite instance %d >= proposed granite instance %d", fs.lastGraniteInstanceNumber, fc.GraniteDecision.InstanceNumber)
	}

	// TODO(jie): Update fields in fs

	return nil
}
