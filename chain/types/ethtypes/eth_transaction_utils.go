package ethtypes

import (
	"fmt"

	"github.com/filecoin-project/lotus/chain/types"
)

func ToSignedMessage(data []byte) (*types.SignedMessage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	if data[0] > 0x7f {
		// legacy transaction
		tx, err := parseLegacyTx(data)
		if err != nil {
			return nil, err
		}
		return tx.ToSignedMessage()
	}

	if data[0] == 1 {
		// EIP-2930
		return nil, fmt.Errorf("EIP-2930 transaction is not supported")
	}

	if data[0] == Eip1559TxType {
		// EIP-1559
		tx, err := parseEip1559Tx(data)
		if err != nil {
			return nil, err
		}
		return tx.ToSignedMessage()
	}

	return nil, fmt.Errorf("unsupported transaction type")
}
