package vm

import (
	"strings"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func TestReservationStatusToErrorMapping(t *testing.T) {
	testCases := []struct {
		code int32
		err  error
	}{
		{code: 0, err: nil},
		{code: 1, err: ErrReservationsNotImplemented},
		{code: 2, err: ErrReservationsInsufficientFunds},
		{code: 3, err: ErrReservationsSessionOpen},
		{code: 4, err: ErrReservationsSessionClosed},
		{code: 5, err: ErrReservationsNonZeroRemainder},
		{code: 6, err: ErrReservationsPlanTooLarge},
		{code: 7, err: ErrReservationsOverflow},
		{code: 8, err: ErrReservationsInvariantViolation},
	}

	for _, tc := range testCases {
		got := reservationStatusToError(tc.code)
		if tc.err == nil {
			if got != nil {
				t.Fatalf("code %d: expected nil error, got %v", tc.code, got)
			}
			continue
		}

		if got != tc.err {
			t.Fatalf("code %d: expected error %v, got %v", tc.code, tc.err, got)
		}
	}

	unknown := reservationStatusToError(99)
	if unknown == nil {
		t.Fatalf("expected non-nil error for unknown status code")
	}
}

func TestReservationPlanCBORRoundTrip(t *testing.T) {
	addr1, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating address 1: %v", err)
	}
	addr2, err := address.NewIDAddress(200)
	if err != nil {
		t.Fatalf("creating address 2: %v", err)
	}

	plan := map[address.Address]abi.TokenAmount{
		addr1: abi.NewTokenAmount(123),
		addr2: abi.NewTokenAmount(456),
	}

	encoded, err := encodeReservationPlanCBOR(plan)
	if err != nil {
		t.Fatalf("encodeReservationPlanCBOR failed: %v", err)
	}

	decoded, err := decodeReservationPlanCBOR(encoded)
	if err != nil {
		t.Fatalf("decodeReservationPlanCBOR failed: %v", err)
	}

	if len(decoded) != len(plan) {
		t.Fatalf("decoded plan length %d != original %d", len(decoded), len(plan))
	}

	for addr, amt := range plan {
		got, ok := decoded[addr]
		if !ok {
			t.Fatalf("missing entry for address %s", addr)
		}

		if amt.String() != got.String() {
			t.Fatalf("amount mismatch for %s: expected %s, got %s", addr, amt.String(), got.String())
		}
	}
}

func TestReservationPlanCBORDecodeDuplicateSender(t *testing.T) {
	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating address: %v", err)
	}

	amount := abi.NewTokenAmount(123)
	amountBytes, err := amount.Bytes()
	if err != nil {
		t.Fatalf("serializing amount: %v", err)
	}

	entry := [][]byte{addr.Bytes(), amountBytes}
	entries := [][][]byte{entry, entry}

	encoded, err := cbor.DumpObject(entries)
	if err != nil {
		t.Fatalf("DumpObject failed: %v", err)
	}

	_, err = decodeReservationPlanCBOR(encoded)
	if err == nil {
		t.Fatalf("expected error when decoding duplicate sender in reservation plan")
	}
	if got, want := err.Error(), "duplicate sender"; !strings.Contains(got, want) {
		t.Fatalf("expected error to contain %q, got %q", want, got)
	}
}

func TestReservationPlanCBORDecodeInvalidEntryLength(t *testing.T) {
	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating address: %v", err)
	}

	// Entry with only the address bytes should be rejected.
	entry := [][]byte{addr.Bytes()}
	entries := [][][]byte{entry}

	encoded, err := cbor.DumpObject(entries)
	if err != nil {
		t.Fatalf("DumpObject failed: %v", err)
	}

	_, err = decodeReservationPlanCBOR(encoded)
	if err == nil {
		t.Fatalf("expected error when decoding entry with invalid length")
	}
	if got, want := err.Error(), "invalid reservation entry length"; !strings.Contains(got, want) {
		t.Fatalf("expected error to contain %q, got %q", want, got)
	}
}
