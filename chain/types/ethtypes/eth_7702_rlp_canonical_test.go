package ethtypes

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
)

// Helper builds a minimal 0x04 tx using manual RLP items to inject a non-canonical integer
// encoding (leading zero) for the specified tuple index.
func buildTxWithLeadingZeroInt(t *testing.T, tupleIndex int) []byte {
	t.Helper()
	var to EthAddress
	copy(to[:], mustHex(t, "0x1111111111111111111111111111111111111111"))
	var authAddr EthAddress
	copy(authAddr[:], mustHex(t, "0x2222222222222222222222222222222222222222"))

	// Outer fields (typed transaction body without the type prefix)
	chainId, _ := formatInt(1)
	nonce, _ := formatInt(1)
	maxPrio, _ := formatBigInt(big.NewInt(1))
	maxFee, _ := formatBigInt(big.NewInt(1))
	gasLimit, _ := formatInt(21000)
	value, _ := formatBigInt(big.NewInt(0))
	input := []byte{}

	// Authorization tuple elements; default canonical encodings
	ai, _ := formatInt(1)
	ni, _ := formatInt(0)
	yp, _ := formatInt(0)
	ri, _ := formatBigInt(big.NewInt(1))
	si, _ := formatBigInt(big.NewInt(1))

	// Replace target field with a non-canonical encoding: add a leading 0x00 byte.
	leadingZero := func(b []byte) []byte { return append([]byte{0x00}, b...) }
	switch tupleIndex {
	case 0:
		ai = leadingZero(ai)
	case 2:
		ni = leadingZero(ni)
	case 3:
		yp = leadingZero(yp)
	default:
		t.Fatalf("unsupported tuple index: %d", tupleIndex)
	}

	// Construct list
	base := []interface{}{
		chainId,
		nonce,
		maxPrio,
		maxFee,
		gasLimit,
		formatEthAddr(&to),
		value,
		input,
		[]interface{}{}, // accessList
		[]interface{}{[]interface{}{ai, authAddr[:], ni, yp, ri, si}},
	}
	sig, _ := packSigFields(big.NewInt(0), big.NewInt(1), big.NewInt(1))
	payload, err := EncodeRLP(append(base, sig...))
	require.NoError(t, err)
	return append([]byte{EIP7702TxType}, payload...)
}

func TestEIP7702_RLPLeadingZero_ChainIDRejected(t *testing.T) {
	enc := buildTxWithLeadingZeroInt(t, 0)
	_, err := parseEip7702Tx(enc)
	require.Error(t, err)
}

func TestEIP7702_RLPLeadingZero_YParityRejected(t *testing.T) {
	enc := buildTxWithLeadingZeroInt(t, 3)
	_, err := parseEip7702Tx(enc)
	require.Error(t, err)
}
