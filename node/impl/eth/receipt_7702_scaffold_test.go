package eth

import (
	"context"
	"testing"

	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-state-types/big"

	ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestAdjustReceiptForDelegation_FromAuthList(t *testing.T) {
	// Build a tx with an authorizationList containing one delegate address
	var delegate ethtypes.EthAddress
	for i := range delegate {
		delegate[i] = 0xAB
	}
	tx := ethtypes.EthTx{
		Type: ethtypes.EthUint64(ethtypes.EIP7702TxType),
		AuthorizationList: []ethtypes.EthAuthorization{{
			ChainID: 1,
			Address: delegate,
			Nonce:   0,
			YParity: 0,
			R:       ethtypes.EthBigInt(big.NewInt(1)),
			S:       ethtypes.EthBigInt(big.NewInt(1)),
		}},
	}
	r := ethtypes.EthTxReceipt{}
	adjustReceiptForDelegation(context.TODO(), &r, tx)
	if len(r.DelegatedTo) != 1 {
		t.Fatalf("expected 1 delegatedTo entry, got %d", len(r.DelegatedTo))
	}
	if r.DelegatedTo[0] != delegate {
		t.Fatalf("delegatedTo mismatch: got %s", r.DelegatedTo[0].String())
	}
}

func TestAdjustReceiptForDelegation_FromSyntheticLog(t *testing.T) {
	// Build a tx without auth list, but with a synthetic log containing
	// topic0=keccak("Delegated(address)") and ABI-encoded 32-byte data
	var topic0 ethtypes.EthHash
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("Delegated(address)"))
	sum := h.Sum(nil)
	copy(topic0[:], sum)

	var delegate ethtypes.EthAddress
	for i := range delegate {
		delegate[i] = 0xCD
	}

	// ABI-encode the address into a 32-byte word (right-aligned).
	var data32 [32]byte
	copy(data32[12:], delegate[:])
	lg := ethtypes.EthLog{
		Topics: []ethtypes.EthHash{topic0},
		Data:   ethtypes.EthBytes(data32[:]),
	}
	r := ethtypes.EthTxReceipt{Logs: []ethtypes.EthLog{lg}}
	tx := ethtypes.EthTx{Type: ethtypes.EthUint64(ethtypes.EIP7702TxType)}

	adjustReceiptForDelegation(context.TODO(), &r, tx)
	if len(r.DelegatedTo) != 1 {
		t.Fatalf("expected 1 delegatedTo entry, got %d", len(r.DelegatedTo))
	}
	if r.DelegatedTo[0] != delegate {
		t.Fatalf("delegatedTo mismatch: got %s", r.DelegatedTo[0].String())
	}
}

func TestAdjustReceiptForDelegation_PrefersAuthList(t *testing.T) {
	// If both auth list and synthetic log present, prefer auth list
	var authAddr ethtypes.EthAddress
	for i := range authAddr {
		authAddr[i] = 0xEF
	}
	tx := ethtypes.EthTx{
		Type: ethtypes.EthUint64(ethtypes.EIP7702TxType),
		AuthorizationList: []ethtypes.EthAuthorization{{
			ChainID: 1, Address: authAddr, Nonce: 0, YParity: 0,
			R: ethtypes.EthBigInt(big.NewInt(1)), S: ethtypes.EthBigInt(big.NewInt(1)),
		}},
	}
	// Also add a conflicting synthetic log for a different address
	var topic0 ethtypes.EthHash
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("Delegated(address)"))
	copy(topic0[:], h.Sum(nil))
	var other ethtypes.EthAddress
	for i := range other {
		other[i] = 0xCD
	}
	var data32 [32]byte
	copy(data32[12:], other[:])
	r := ethtypes.EthTxReceipt{Logs: []ethtypes.EthLog{{Topics: []ethtypes.EthHash{topic0}, Data: ethtypes.EthBytes(data32[:])}}}

	adjustReceiptForDelegation(context.TODO(), &r, tx)
	if len(r.DelegatedTo) != 1 || r.DelegatedTo[0] != authAddr {
		t.Fatalf("expected delegatedTo from auth list; got %v", r.DelegatedTo)
	}
}

func TestAdjustReceiptForDelegation_NoopWhenNoData(t *testing.T) {
	// When tx has no authorization list and no logs, DelegatedTo remains empty
	tx := ethtypes.EthTx{Type: ethtypes.EthUint64(ethtypes.EIP7702TxType)}
	r := ethtypes.EthTxReceipt{}
	adjustReceiptForDelegation(context.TODO(), &r, tx)
	if len(r.DelegatedTo) != 0 {
		t.Fatalf("expected no delegatedTo; got %v", r.DelegatedTo)
	}
}

func TestAdjustReceiptForDelegation_MultipleSyntheticLogs(t *testing.T) {
	// Two logs with the Delegated topic should yield two delegates
	var topic0 ethtypes.EthHash
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("Delegated(address)"))
	copy(topic0[:], h.Sum(nil))

	mkAddr := func(b byte) ethtypes.EthAddress {
		var a ethtypes.EthAddress
		for i := range a {
			a[i] = b
		}
		return a
	}
	a1 := mkAddr(0x01)
	a2 := mkAddr(0x02)

	var d1, d2 [32]byte
	copy(d1[12:], a1[:])
	copy(d2[12:], a2[:])
	logs := []ethtypes.EthLog{
		{Topics: []ethtypes.EthHash{topic0}, Data: ethtypes.EthBytes(d1[:])},
		{Topics: []ethtypes.EthHash{topic0}, Data: ethtypes.EthBytes(d2[:])},
		// a malformed log should be ignored
		{Topics: []ethtypes.EthHash{topic0}, Data: ethtypes.EthBytes([]byte{0xAA})},
		// wrong topic ignored
		{Topics: []ethtypes.EthHash{{}}, Data: ethtypes.EthBytes(d1[:])},
	}
	r := ethtypes.EthTxReceipt{Logs: logs}
	tx := ethtypes.EthTx{Type: ethtypes.EthUint64(ethtypes.EIP7702TxType)}
	adjustReceiptForDelegation(context.TODO(), &r, tx)
	if len(r.DelegatedTo) != 2 {
		t.Fatalf("expected 2 delegatedTo entries, got %d", len(r.DelegatedTo))
	}
	if r.DelegatedTo[0] != a1 || r.DelegatedTo[1] != a2 {
		t.Fatalf("unexpected delegates: %v", r.DelegatedTo)
	}
}

func TestAdjustReceiptForDelegation_TakesLast20Bytes(t *testing.T) {
	// Data longer than 20 bytes -> use last 20 bytes
	var topic0 ethtypes.EthHash
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("Delegated(address)"))
	copy(topic0[:], h.Sum(nil))

	// Construct data = 12 bytes prefix + 20-byte address
	prefix := make([]byte, 12)
	var delegate ethtypes.EthAddress
	for i := range delegate {
		delegate[i] = 0x5A
	}
	data := append(prefix, delegate[:]...)
	lg := ethtypes.EthLog{Topics: []ethtypes.EthHash{topic0}, Data: ethtypes.EthBytes(data)}

	r := ethtypes.EthTxReceipt{Logs: []ethtypes.EthLog{lg}}
	tx := ethtypes.EthTx{Type: ethtypes.EthUint64(ethtypes.EIP7702TxType)}
	adjustReceiptForDelegation(context.TODO(), &r, tx)
	if len(r.DelegatedTo) != 1 || r.DelegatedTo[0] != delegate {
		t.Fatalf("expected delegatedTo to take last 20 bytes; got %v", r.DelegatedTo)
	}
}
