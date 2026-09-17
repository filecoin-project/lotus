package stmgr

import (
	"math"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
)

func TestConfidenceReached(t *testing.T) {
	tests := []struct {
		name       string
		current    abi.ChainEpoch
		candidate  abi.ChainEpoch
		confidence uint64
		want       bool
	}{
		{"zero confidence", 10, 10, 0, true},
		{"exact confidence", 15, 10, 5, true},
		{"negative candidate", 0, -1, 1, false},
		{"insufficient confidence", 14, 10, 5, false},
		{"candidate above current", 9, 10, 0, false},
		{"uint64 confidence does not overflow", abi.ChainEpoch(math.MaxInt64), 0, math.MaxUint64, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := confidenceReached(test.current, test.candidate, test.confidence); got != test.want {
				t.Fatalf("confidenceReached(%d, %d, %d) = %t, want %t", test.current, test.candidate, test.confidence, got, test.want)
			}
		})
	}
}
