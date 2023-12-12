package consensus

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

type FinalityState struct {
	lastFinalizedEpoch        int64
	lastGraniteInstanceNumber int64
	powerTable                types.PowerTable
}

func (fs *FinalityState) ValidateFinalityCertificate(fc *types.FinalityCertificate) error {
	if fc == nil {
		// TODO(jie): 换成log.Info()?
		fmt.Println("Empty FinalityCertificate. Skip.")
		return nil
	}

	// TODO(jie): Validate voter's identity and total power

	// TODO(jie): Validate blssignature

	if fs.lastFinalizedEpoch >= fc.GraniteDecision.Epoch {
		return xerrors.Errorf("last finalized epoch %d >= proposed finalized epoch %d", fs.lastFinalizedEpoch, fc.GraniteDecision.Epoch)
	}
	if fs.lastGraniteInstanceNumber >= fc.GraniteDecision.InstanceNumber {
		return xerrors.Errorf("last granite instance %d >= proposed granite instance %d", fs.lastGraniteInstanceNumber, fc.GraniteDecision.InstanceNumber)
	}

	fmt.Println("Successfully validated finality certificate")

	fs.lastFinalizedEpoch = fc.GraniteDecision.Epoch
	fs.lastGraniteInstanceNumber = fc.GraniteDecision.InstanceNumber

	// TODO(jie): Update other fields in finality state.

	return nil
}
