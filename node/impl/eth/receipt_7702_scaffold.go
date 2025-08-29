package eth

import (
    "context"
    "github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// adjustReceiptForDelegation is a placeholder to capture receipt adjustments
// for EIP-7702 delegated execution. For transactions that apply delegation and
// then execute delegate code, log attribution and any additional receipt fields
// may need to reflect the delegate address.
//
// This function should be called from EthGetTransactionReceipt once actor/FVM
// integration exists and we can detect 7702 flows.
func adjustReceiptForDelegation(_ context.Context, receipt *ethtypes.EthTxReceipt) {
    // TODO: when a 7702 tx is detected, ensure receipt.From/To/logs context align
    // with delegate execution semantics and authorizationList echoes back.
}

