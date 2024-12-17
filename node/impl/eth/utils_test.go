package eth

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
)

func TestABIEncoding(t *testing.T) {
	// Generated from https://abi.hashex.org/
	const expected = "000000000000000000000000000000000000000000000000000000000000001600000000000000000000000000000000000000000000000000000000000000510000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000001b1111111111111111111020200301000000044444444444444444010000000000"
	const data = "111111111111111111102020030100000004444444444444444401"

	expectedBytes, err := hex.DecodeString(expected)
	require.NoError(t, err)

	dataBytes, err := hex.DecodeString(data)
	require.NoError(t, err)

	require.Equal(t, expectedBytes, encodeAsABIHelper(22, 81, dataBytes))
}

func TestDecodePayload(t *testing.T) {
	// "empty"
	b, err := decodePayload(nil, 0)
	require.NoError(t, err)
	require.Empty(t, b)

	// raw empty
	_, err = decodePayload(nil, uint64(multicodec.Raw))
	require.NoError(t, err)
	require.Empty(t, b)

	// raw non-empty
	b, err = decodePayload([]byte{1}, uint64(multicodec.Raw))
	require.NoError(t, err)
	require.EqualValues(t, b, []byte{1})

	// Invalid cbor bytes
	_, err = decodePayload(nil, uint64(multicodec.DagCbor))
	require.Error(t, err)

	// valid cbor bytes
	var w bytes.Buffer
	require.NoError(t, cbg.WriteByteArray(&w, []byte{1}))
	b, err = decodePayload(w.Bytes(), uint64(multicodec.DagCbor))
	require.NoError(t, err)
	require.EqualValues(t, b, []byte{1})

	// regular cbor also works.
	b, err = decodePayload(w.Bytes(), uint64(multicodec.Cbor))
	require.NoError(t, err)
	require.EqualValues(t, b, []byte{1})

	// random codec should fail
	_, err = decodePayload(w.Bytes(), 42)
	require.Error(t, err)
}
