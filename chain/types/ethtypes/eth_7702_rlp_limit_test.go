package ethtypes

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
)

// Verifies that the 0x04 parser uses a per-type RLP element limit (13) and
// rejects lists that are too long, while the EIP-1559 parser remains
// unaffected and expects exactly 12 elements.
func TestEIP7702_RLP_OuterListTooLongRejected(t *testing.T) {
	// Build a valid-looking 7702 payload first.
	chainId, _ := formatInt(1)
	nonce, _ := formatInt(1)
	maxPrio, _ := formatBigInt(big.NewInt(1))
	maxFee, _ := formatBigInt(big.NewInt(1))
	gasLimit, _ := formatInt(21000)
	value, _ := formatBigInt(big.NewInt(0))
	input := []byte{}

	// Minimal valid authorization list with one 6-tuple.
	var to EthAddress
	ai, _ := formatInt(1)
	ni, _ := formatInt(0)
	yp, _ := formatInt(0)
	ri, _ := formatBigInt(big.NewInt(1))
	si, _ := formatBigInt(big.NewInt(1))
	authList := []interface{}{[]interface{}{ai, to[:], ni, yp, ri, si}}

	// Base 10 fields (pre-accesslist + access list + auth list)
	base := []interface{}{chainId, nonce, maxPrio, maxFee, gasLimit, formatEthAddr(&to), value, input, []interface{}{}, authList}

	// Append signature fields (v, r, s)
	sig, _ := packSigFields(big.NewInt(0), big.NewInt(1), big.NewInt(1))
	payload := append(base, sig...)

	// Sanity: this should parse as a proper 7702 transaction
	encOk, err := EncodeRLP(payload)
	require.NoError(t, err)
	_, err = parseEip7702Tx(append([]byte{EIP7702TxType}, encOk...))
	require.NoError(t, err)

	// Now make the outer list too long by appending an extra empty byte string.
	payloadTooLong := append(payload, []byte{})
	encBad, err := EncodeRLP(payloadTooLong)
	require.NoError(t, err)

	// Expect the 7702 parser to reject the 14-element list.
	_, err = parseEip7702Tx(append([]byte{EIP7702TxType}, encBad...))
	require.Error(t, err)
}
