package proof

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	ffi "github.com/filecoin-project/filecoin-ffi"
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

	t.Run("json-type-check", func(t *testing.T) {
		// deserialize the proof with Go types
		var proof1 Commit1OutRaw
		err = json.Unmarshal(c1out, &proof1)
		require.NoError(t, err)

		// serialize the proof to JSON
		proof1out, err := json.Marshal(proof1)
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
}
