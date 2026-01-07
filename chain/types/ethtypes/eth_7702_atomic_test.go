package ethtypes

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	abi2 "github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

func TestEIP7702_ToUnsignedFilecoinMessageAtomic_ShapesAndMethod(t *testing.T) {
	// Enable feature flag and set EthAccount.ApplyAndCall addr
	Eip7702FeatureEnabled = true
	defer func() { Eip7702FeatureEnabled = false }()
	EthAccountApplyAndCallActorAddr, _ = address.NewIDAddress(999)

	// Assemble a simple tx with one auth and call fields
	var to EthAddress
	for i := range to {
		to[i] = 0xAA
	}
	var authAddr EthAddress
	for i := range authAddr {
		authAddr[i] = 0xBB
	}
	tx := &Eth7702TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Nonce:                1,
		To:                   &to,
		Value:                big.NewInt(12345),
		Input:                []byte{0xde, 0xad, 0xbe, 0xef},
		MaxFeePerGas:         big.NewInt(1),
		MaxPriorityFeePerGas: big.NewInt(1),
		GasLimit:             50000,
		AuthorizationList: []EthAuthorization{{
			ChainID: EthUint64(buildconstants.Eip155ChainId),
			Address: authAddr,
			Nonce:   7,
			YParity: 0,
			R:       EthBigInt(big.NewInt(1)),
			S:       EthBigInt(big.NewInt(2)),
		}},
	}
	from, _ := address.NewIDAddress(1001)
	msg, err := tx.ToUnsignedFilecoinMessageAtomic(from)
	require.NoError(t, err)
	require.EqualValues(t, abi2.MethodNum(MethodHash("ApplyAndCall")), msg.Method)

	// Decode params shape: [ [tuple...], [to(20b), value(bytes), input(bytes)] ]
	r := cbg.NewCborReader(bytes.NewReader(msg.Params))
	maj, l, err := r.ReadHeader()
	require.NoError(t, err)
	require.EqualValues(t, cbg.MajArray, maj)
	require.EqualValues(t, 2, l)

	// Inner list length
	maj, innerLen, err := r.ReadHeader()
	require.NoError(t, err)
	require.EqualValues(t, cbg.MajArray, maj)
	require.EqualValues(t, 1, innerLen)
	// Consume inner tuples generically
	for i := 0; i < int(innerLen); i++ {
		// tuple header
		_, tlen, err := r.ReadHeader()
		require.NoError(t, err)
		require.EqualValues(t, 6, tlen)
		// chain_id
		_, _, err = r.ReadHeader()
		require.NoError(t, err)
		// address
		_, blen, err := r.ReadHeader()
		require.NoError(t, err)
		require.EqualValues(t, uint64(20), blen)
		// consume address bytes
		if blen > 0 {
			tmp1 := make([]byte, blen)
			_, err = r.Read(tmp1)
			require.NoError(t, err)
		}
		// nonce
		_, _, err = r.ReadHeader()
		require.NoError(t, err)
		// y_parity
		_, _, err = r.ReadHeader()
		require.NoError(t, err)
		// r
		_, blen, err = r.ReadHeader()
		require.NoError(t, err)
		require.GreaterOrEqual(t, blen, uint64(1))
		tmp2 := make([]byte, blen)
		_, err = r.Read(tmp2)
		require.NoError(t, err)
		// s
		_, blen, err = r.ReadHeader()
		require.NoError(t, err)
		require.GreaterOrEqual(t, blen, uint64(1))
		tmp3 := make([]byte, blen)
		_, err = r.Read(tmp3)
		require.NoError(t, err)
	}

	// Call tuple
	maj, clen, err := r.ReadHeader()
	require.NoError(t, err)
	require.EqualValues(t, cbg.MajArray, maj)
	require.EqualValues(t, uint64(3), clen)
	// to
	maj, blen, err := r.ReadHeader()
	require.NoError(t, err)
	require.EqualValues(t, cbg.MajByteString, maj)
	require.EqualValues(t, uint64(20), blen)
	if blen > 0 {
		tmp4 := make([]byte, blen)
		_, err = r.Read(tmp4)
		require.NoError(t, err)
	}
	// value
	maj, blen, err = r.ReadHeader()
	require.NoError(t, err)
	require.EqualValues(t, cbg.MajByteString, maj)
	require.GreaterOrEqual(t, blen, uint64(1))
	tmp5 := make([]byte, blen)
	_, err = r.Read(tmp5)
	require.NoError(t, err)
	// input
	maj, blen, err = r.ReadHeader()
	require.NoError(t, err)
	require.EqualValues(t, cbg.MajByteString, maj)
	require.EqualValues(t, uint64(4), blen)
}
