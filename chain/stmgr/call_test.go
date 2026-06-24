package stmgr

import (
	"context"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestStateCallGasLimitDefaultAndOverride(t *testing.T) {
	sm := &StateManager{}
	if got := sm.StateCallGasLimit(); got != buildconstants.BlockGasLimit {
		t.Fatalf("expected default StateCall gas limit %d, got %d", buildconstants.BlockGasLimit, got)
	}

	sm.SetStateCallGasLimit(123)
	if got := sm.StateCallGasLimit(); got != 123 {
		t.Fatalf("expected configured StateCall gas limit 123, got %d", got)
	}

	sm.SetStateCallGasLimit(-1)
	if got := sm.StateCallGasLimit(); got != buildconstants.BlockGasLimit {
		t.Fatalf("expected negative StateCall gas limit to fall back to %d, got %d", buildconstants.BlockGasLimit, got)
	}
}

func TestCallOnStateRejectsGasLimitOverCap(t *testing.T) {
	sm := &StateManager{}
	sm.SetStateCallGasLimit(10)

	_, err := sm.CallOnState(context.Background(), cid.Undef, &types.Message{GasLimit: 11}, nil)
	if err == nil {
		t.Fatal("expected StateCall gas limit above cap to be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds configured cap 10") {
		t.Fatalf("expected gas cap error, got %q", err)
	}
}
