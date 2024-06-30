package proof

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	ffi "github.com/filecoin-project/filecoin-ffi"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundtripPorepVproof(t *testing.T) {
	// seal a sector (random data)
	testDir := t.TempDir()

	paddedSize := abi.PaddedPieceSize(2048)
	unpaddedSize := paddedSize.Unpadded()

	// generate a random sector
	sectorData := make([]byte, unpaddedSize)
	_, err := rand.Read(sectorData)
	require.NoError(t, err)

	unsFile := filepath.Join(testDir, "unsealed")
	cacheFile := filepath.Join(testDir, "cache")
	sealedFile := filepath.Join(testDir, "sealed")

	{
		err = os.MkdirAll(cacheFile, 0755)
		require.NoError(t, err)
		f, err := os.Create(sealedFile)
		require.NoError(t, err)
		err = f.Close()
		require.NoError(t, err)
	}

	commd, err := BuildTreeD(bytes.NewReader(sectorData), true, unsFile, 2048)
	require.NoError(t, err)
	fmt.Println("D:", commd)

	err = os.Truncate(unsFile, int64(paddedSize)) // prefix contains exactly unsealed data
	require.NoError(t, err)

	spt := abi.RegisteredSealProof_StackedDrg2KiBV1_1
	num := abi.SectorNumber(234)
	miner := abi.ActorID(123)

	var ticket [32]byte
	_, err = rand.Read(ticket[:])
	require.NoError(t, err)
	ticket[31] &= 0x3f // fr32

	pieces := []abi.PieceInfo{{
		Size:     paddedSize,
		PieceCID: commd,
	}}

	p1o, err := ffi.SealPreCommitPhase1(spt, cacheFile, unsFile, sealedFile, num, miner, ticket[:], pieces)
	require.NoError(t, err)

	sealed, unsealed, err := ffi.SealPreCommitPhase2(p1o, cacheFile, sealedFile)
	require.NoError(t, err)
	_ = sealed
	_ = unsealed

	// generate a proof
	var seed [32]byte
	_, err = rand.Read(seed[:])
	require.NoError(t, err)
	seed[31] &= 0x3f // fr32

	c1out, err := ffi.SealCommitPhase1(spt, sealed, unsealed, cacheFile, sealedFile, num, miner, ticket[:], seed[:], pieces)
	require.NoError(t, err)

	// deserialize the proof with Go types
	var realVProof Commit1OutRaw
	err = json.Unmarshal(c1out, &realVProof)
	require.NoError(t, err)

	t.Run("json-roundtrip-check", func(t *testing.T) {
		// serialize the proof to JSON
		proof1out, err := json.Marshal(realVProof)
		require.NoError(t, err)

		// check that the JSON is as expected
		var rustObj, goObj map[string]interface{}
		err = json.Unmarshal(c1out, &rustObj)
		require.NoError(t, err)
		err = json.Unmarshal(proof1out, &goObj)
		require.NoError(t, err)

		diff := cmp.Diff(rustObj, goObj)
		if !cmp.Equal(rustObj, goObj) {
			t.Errorf("proof mismatch: %s", diff)
		}

		require.True(t, cmp.Equal(rustObj, goObj))
	})

	t.Run("check-toplevel", func(t *testing.T) {
		/*
			type Commit1OutRaw struct {
				CommD           Commitment                `json:"comm_d"` // CHECK
				CommR           Commitment                `json:"comm_r"` // CHECK
				RegisteredProof StringRegisteredProofType `json:"registered_proof"` // CHECK
				ReplicaID       Commitment                `json:"replica_id"` // CHECK
				Seed            Ticket                    `json:"seed"` // CHECK
				Ticket          Ticket                    `json:"ticket"` // CHECK

				VanillaProofs map[StringRegisteredProofType][][]VanillaStackedProof `json:"vanilla_proofs"`
			}
		*/

		rawCommD, err := commcid.CIDToDataCommitmentV1(unsealed)
		require.NoError(t, err)
		rawCommR, err := commcid.CIDToReplicaCommitmentV1(sealed)
		require.NoError(t, err)

		require.Equal(t, realVProof.CommD, Commitment(rawCommD))
		require.Equal(t, realVProof.CommR, Commitment(rawCommR))

		require.Equal(t, realVProof.RegisteredProof, StringRegisteredProofType("StackedDrg2KiBV1_1"))

		replicaID, err := spt.ReplicaId(miner, num, ticket[:], realVProof.CommD[:])
		require.NoError(t, err)
		require.Equal(t, realVProof.ReplicaID, Commitment(replicaID))

		require.Equal(t, realVProof.Seed, Ticket(seed))
		require.Equal(t, realVProof.Ticket, Ticket(ticket))
	})

	t.Run("check-d-proofs", func(t *testing.T) {
		require.Len(t, realVProof.VanillaProofs, 1)

		expected := extractDProofs(realVProof.VanillaProofs["StackedDrg2KiBV1"]) // fun fact: this doesn't have _1 in the name...

		// todo compute + extract
		var actual [][]MerkleProof[Sha256Domain]

		requireNoDiff(t, expected, actual)
	})
}

func extractDProofs(vp [][]VanillaStackedProof) [][]MerkleProof[Sha256Domain] {
	var out [][]MerkleProof[Sha256Domain]
	for _, v := range vp {
		var proofs []MerkleProof[Sha256Domain]
		for _, p := range v {
			proofs = append(proofs, p.CommDProofs)
		}
		out = append(out, proofs)
	}
	return out
}

func requireNoDiff(t *testing.T, rustObj, goObj interface{}) {
	diff := cmp.Diff(rustObj, goObj)
	if !cmp.Equal(rustObj, goObj) {
		t.Errorf("proof mismatch: %s", diff)
	}

	require.True(t, cmp.Equal(rustObj, goObj))
}
