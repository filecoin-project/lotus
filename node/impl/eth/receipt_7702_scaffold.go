package eth

import (
	"context"
	"encoding/hex"

	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// adjustReceiptForDelegation is a placeholder to capture receipt adjustments
// for EIP-7702 delegated execution. For transactions that apply delegation and
// then execute delegate code, log attribution and any additional receipt fields
// may need to reflect the delegate address.
//
// This function should be called from EthGetTransactionReceipt once actor/FVM
// integration exists and we can detect 7702 flows.
func adjustReceiptForDelegation(_ context.Context, receipt *ethtypes.EthTxReceipt, tx ethtypes.EthTx) {
	// Minimal attribution for EIP-7702:
	// - AuthorizationList is already echoed via tx view in newEthTxReceipt.
	// - We leave From/To unchanged to preserve caller/callee semantics.
	// - Logs attribution remains based on emitter addresses within logs.
	//
	// We also surface an optional DelegatedTo array containing the delegate addresses
	// referenced by authorization tuples for EIP-7702 transactions.
	if receipt == nil {
		return
	}
	if len(tx.AuthorizationList) > 0 {
		// Best-effort extraction of delegate addresses from the tuples.
		delegated := make([]ethtypes.EthAddress, 0, len(tx.AuthorizationList))
		for _, a := range tx.AuthorizationList {
			delegated = append(delegated, a.Address)
		}
		if len(delegated) > 0 {
			receipt.DelegatedTo = delegated
		}
	}

	// For delegated execution via CALL→EOA, detect the synthetic EVM log topic and extract the
	// authority address from the data blob.
	// Topic0: keccak256("Delegated(address)")
	if len(receipt.DelegatedTo) == 0 && len(receipt.Logs) > 0 {
		h := sha3.NewLegacyKeccak256()
		_, _ = h.Write([]byte("Delegated(address)"))
		topic := h.Sum(nil)
		for _, lg := range receipt.Logs {
			if len(lg.Topics) == 0 {
				continue
			}
			// Compare topic0 bytes
			if hex.EncodeToString(lg.Topics[0][:]) != hex.EncodeToString(topic) {
				continue
			}
			// Data carries an ABI-encoded 32-byte word for the authority address.
			// We extract the last 20 bytes to form the EthAddress.
			if len(lg.Data) >= 20 {
				var addr ethtypes.EthAddress
				copy(addr[:], lg.Data[len(lg.Data)-20:])
				receipt.DelegatedTo = append(receipt.DelegatedTo, addr)
			}
		}
	}
}
