package eth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/types"
	ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// Verifies that newEthTxReceipt carries over authorizationList from the tx view
// for type-0x04 (EIP-7702) transactions.
func TestNewEthTxReceipt_CarriesAuthorizationList(t *testing.T) {
	// Minimal EthTx with one authorization tuple
	var from, to ethtypes.EthAddress
	for i := 0; i < len(from); i++ {
		from[i] = 0x11
	}
	for i := 0; i < len(to); i++ {
		to[i] = 0x22
	}

	tx := ethtypes.EthTx{
		ChainID: ethtypes.EthUint64(1),
		From:    from,
		To:      &to,
		Type:    ethtypes.EthUint64(ethtypes.EIP7702TxType),
		Gas:     ethtypes.EthUint64(21000),
	}
	// Populate fee fields to satisfy GasFeeCap()/GasPremium()
	maxFee := ethtypes.EthBigInt(big.NewInt(1))
	tip := ethtypes.EthBigInt(big.NewInt(1))
	tx.MaxFeePerGas = &maxFee
	tx.MaxPriorityFeePerGas = &tip

	// Attach a single authorization tuple
	var authAddr ethtypes.EthAddress
	for i := 0; i < len(authAddr); i++ {
		authAddr[i] = 0xAB
	}
	tx.AuthorizationList = []ethtypes.EthAuthorization{
		{ChainID: 1, Address: authAddr, Nonce: 0, YParity: 0, R: ethtypes.EthBigInt(big.NewInt(1)), S: ethtypes.EthBigInt(big.NewInt(1))},
	}

	// Dummy receipt: success, no events root so logs path not exercised
	msgReceipt := types.MessageReceipt{ExitCode: 0, GasUsed: 21000}

	rcpt, err := newEthTxReceipt(context.Background(), tx, big.NewInt(0), msgReceipt, nil)
	require.NoError(t, err)
	require.Len(t, rcpt.AuthorizationList, 1)
	require.Equal(t, tx.AuthorizationList[0].Address, rcpt.AuthorizationList[0].Address)
}
