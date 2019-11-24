package actors

import (
	"fmt"
	"github.com/filecoin-project/lotus/build"
	"gotest.tools/assert"
	"testing"
)

func TestProvingPeriodEnd(t *testing.T) {
	assertPPE := func(setPeriodEnd uint64, height uint64, expectEnd uint64) {
		end, _ := ProvingPeriodEnd(setPeriodEnd, height)
		assert.Equal(t, expectEnd, end)
		assert.Equal(t, expectEnd, end)

		fmt.Println(end)
		fmt.Println(end - build.PoStChallangeTime)
	}

	// assumes proving dur of 40 epochs
	assertPPE(185, 147, 185)
}
