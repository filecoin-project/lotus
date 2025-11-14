package consensus

import (
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

// BenchmarkBuildReservationPlan measures the cost of aggregating per-sender
// reservations across a large synthetic tipset. This provides an upper bound
// on the Stage-1 host-side overhead for tipset reservations.
func BenchmarkBuildReservationPlan(b *testing.B) {
	addr1, err := address.NewIDAddress(100)
	if err != nil {
		b.Fatalf("creating addr1: %v", err)
	}
	addr2, err := address.NewIDAddress(200)
	if err != nil {
		b.Fatalf("creating addr2: %v", err)
	}

	const numBlocks = 5
	const msgsPerBlock = 2000 // 10k messages total.

	bms := make([]FilecoinBlockMessages, numBlocks)
	for i := range bms {
		bls := make([]types.ChainMsg, 0, msgsPerBlock)
		for j := 0; j < msgsPerBlock; j++ {
			from := addr1
			if j%2 == 1 {
				from = addr2
			}
			msg := &types.Message{
				From:      from,
				To:        addr2,
				Nonce:     uint64(j),
				Value:     abi.NewTokenAmount(0),
				GasFeeCap: abi.NewTokenAmount(1),
				GasLimit:  1_000_000,
			}
			bls = append(bls, msg)
		}
		bms[i] = FilecoinBlockMessages{
			BlockMessages: store.BlockMessages{
				BlsMessages: bls,
			},
			WinCount: 1,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plan := buildReservationPlan(bms)
		if len(plan) == 0 {
			b.Fatalf("unexpected empty reservation plan")
		}
	}
}

