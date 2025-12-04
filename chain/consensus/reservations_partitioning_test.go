package consensus

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/types"
)

// Note: This test verifies the Strict Sender Partitioning logic implemented in buildReservationPlan.
// It mocks the ChainMsg interface to avoid heavy dependencies, but the package 'consensus' itself
// imports 'vm' which imports 'filecoin-ffi'. Therefore, running this test requires a working
// FFI build environment (libfilcrypto). If the build fails due to CGO/FFI issues, verify the
// logic by inspection or by fixing the FFI bindings.

// mockChainMsg implements types.ChainMsg for testing
type mockChainMsg struct {
	msg *types.Message
	cid cid.Cid
}

func (m *mockChainMsg) VMMessage() *types.Message {
	return m.msg
}

func (m *mockChainMsg) Cid() cid.Cid {
	return m.cid
}

func (m *mockChainMsg) ChainLength() int {
	return 0
}

func TestBuildReservationPlan_StrictPartitioning_Scenarios(t *testing.T) {
	// addresses
	t1, _ := address.NewIDAddress(100)
	t2, _ := address.NewIDAddress(200)
	t3, _ := address.NewIDAddress(300)

	// helper to create a message
	makeMsg := func(from address.Address, nonce uint64, gasLimit int64, gasFeeCap int64) *mockChainMsg {
		m := &types.Message{
			From:      from,
			Nonce:     nonce,
			GasLimit:  gasLimit,
			GasFeeCap: types.NewInt(uint64(gasFeeCap)),
			Value:     big.Zero(),
			Method:    0,
		}
		// derive a fake CID based on fields to ensure uniqueness when we want it
		// We use a dummy CID builder for simplicity or just random CIDs if needed.
		// For this test, we can just assume unique objects = unique CIDs if we were real,
		// but here we need to explicitly set CIDs if we want to test dedupe.
		// Let's just make a unique CID based on nonce/from.
		// Actually, `buildReservationPlan` relies on `m.Cid()`.
		// We'll generate a dummy CID.
		builder := cid.V1Builder{Codec: cid.DagCBOR, MhType: 0x12} // sha2-256
		c, _ := builder.Sum([]byte{byte(from.Protocol()), byte(nonce), byte(gasLimit)})
		return &mockChainMsg{msg: m, cid: c}
	}

	// Helper to create a block from messages
	makeBlock := func(msgs ...*mockChainMsg) FilecoinBlockMessages {
		cms := make([]types.ChainMsg, len(msgs))
		for i, m := range msgs {
			cms[i] = m
		}
		// We put them in BlsMessages for simplicity
		fbm := FilecoinBlockMessages{}
		fbm.BlsMessages = cms
		return fbm
	}

	t.Run("Scenario 1: Single Block, Multiple Senders", func(t *testing.T) {
		m1 := makeMsg(t1, 1, 1000, 10) // Cost 10000
		m2 := makeMsg(t2, 1, 2000, 10) // Cost 20000

		bms := []FilecoinBlockMessages{makeBlock(m1, m2)}
		plan := buildReservationPlan(bms)

		require.Len(t, plan, 2)
		require.Equal(t, int64(10000), plan[t1].Int64())
		require.Equal(t, int64(20000), plan[t2].Int64())
	})

	t.Run("Scenario 2: Split Attack (Block A vs Block B)", func(t *testing.T) {
		// Sender t1 appears in both blocks.
		// Block A (High Precedence)
		m1_A := makeMsg(t1, 1, 1000, 10) // Cost 10000

		// Block B (Low Precedence)
		// Uses same sender t1. Different nonce/message.
		m1_B := makeMsg(t1, 2, 5000, 10) // Cost 50000

		// Sender t2 only in Block B
		m2_B := makeMsg(t2, 1, 2000, 10) // Cost 20000

		bms := []FilecoinBlockMessages{makeBlock(m1_A), makeBlock(m1_B, m2_B)}
		plan := buildReservationPlan(bms)

		// t1: Should ONLY have m1_A cost (10000). m1_B should be ignored.
		require.Contains(t, plan, t1)
		require.Equal(t, int64(10000), plan[t1].Int64())

		// t2: Should have m2_B cost (20000).
		require.Contains(t, plan, t2)
		require.Equal(t, int64(20000), plan[t2].Int64())
	})

	t.Run("Scenario 3: Duplicate Message (Same CID) across Blocks", func(t *testing.T) {
		// m1 is in both A and B.
		m1 := makeMsg(t1, 1, 1000, 10)

		bms := []FilecoinBlockMessages{makeBlock(m1), makeBlock(m1)}
		plan := buildReservationPlan(bms)

		// Should be counted exactly once.
		require.Equal(t, int64(10000), plan[t1].Int64())
	})

	t.Run("Scenario 4: Multiple Messages from Same Sender in Same Block", func(t *testing.T) {
		// t1 sends two messages in Block A.
		m1 := makeMsg(t1, 1, 1000, 10)
		m2 := makeMsg(t1, 2, 1000, 10)

		bms := []FilecoinBlockMessages{makeBlock(m1, m2)}
		plan := buildReservationPlan(bms)

		// Should sum both.
		require.Equal(t, int64(20000), plan[t1].Int64())
	})

	t.Run("Scenario 5: Complex Interleaving", func(t *testing.T) {
		// Block A: t1(m1), t2(m2)
		// Block B: t1(m3), t3(m4)
		// Block C: t2(m5), t3(m6)

		// Expected:
		// t1: From A only (m1). m3 ignored.
		// t2: From A only (m2). m5 ignored.
		// t3: From B (m4). m6 ignored (because t3 seen in B). Wait, m6 is in C. B > C. So m6 ignored.

		m1 := makeMsg(t1, 1, 100, 1)
		m2 := makeMsg(t2, 1, 100, 1)

		m3 := makeMsg(t1, 2, 100, 1) // Ignored (t1 in A)
		m4 := makeMsg(t3, 1, 100, 1) // Counted (t3 new in B)

		m5 := makeMsg(t2, 2, 100, 1) // Ignored (t2 in A)
		m6 := makeMsg(t3, 2, 100, 1) // Ignored (t3 in B)

		bms := []FilecoinBlockMessages{
			makeBlock(m1, m2),
			makeBlock(m3, m4),
			makeBlock(m5, m6),
		}
		plan := buildReservationPlan(bms)

		require.Equal(t, int64(100), plan[t1].Int64())
		require.Equal(t, int64(100), plan[t2].Int64())
		require.Equal(t, int64(100), plan[t3].Int64())
	})
}
