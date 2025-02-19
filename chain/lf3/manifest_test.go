package lf3

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-f3/manifest"
)

//go:embed testdata/contract_manifest_golden.json
var manifestJSONBytes []byte

//go:embed testdata/contract_return.hex
var hexReturn string

func TestContractManifest_ParseEthData(t *testing.T) {
	var manifestGolden manifest.Manifest
	err := json.Unmarshal(manifestJSONBytes, &manifestGolden)
	require.NoError(t, manifestGolden.Validate(), "golden manifest is not valid")

	require.NoErrorf(t, err, "did manifest format change?")
	decodedHex, err := hex.DecodeString(strings.Trim(hexReturn, "\n"))
	require.NoError(t, err, "failed to decode hex string")

	activationEpoch, compressedManifest, err := parseContractReturn(decodedHex)
	require.NoError(t, err, "parseContractReturn failed")

	require.Equal(t, uint64(5000000), activationEpoch, "activationEpoch mismatch")

	manifest, err := decompressManifest(compressedManifest)
	require.NoError(t, err, "decompressManifest failed")

	require.NoError(t, manifest.Validate(), "manifest is not valid")

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err, "failed to marshal manifest")

	require.JSONEq(t, string(manifestJSONBytes), string(manifestJSON), "manifest JSON mismatch")

}
