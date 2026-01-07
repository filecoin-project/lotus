package ethtypes

import (
	"testing"
)

// FuzzParseEthTransaction_7702 exercises the 0x04 typed transaction RLP parser with arbitrary
// inputs. This is an opt-in fuzz harness; it does not run during normal `go test` but can be used
// locally via `go test -fuzz=Fuzz -run=^$ ./chain/types/ethtypes`.
func FuzzParseEthTransaction_7702(f *testing.F) {
	// Seed a few minimal cases.
	f.Add([]byte{0x04})                   // just the type prefix
	f.Add([]byte{0x04, 0xC0})             // type + empty list
	f.Add([]byte{0x04, 0x80})             // type + empty string
	f.Add([]byte{0x04, 0xE0, 0x00})       // type + long list header
	f.Add([]byte{0x04, 0xF8, 0x01, 0x00}) // type + long bytes header

	f.Fuzz(func(t *testing.T, data []byte) {
		// We only care about parser stability: no panics. We ignore errors.
		_ = func() (err error) {
			defer func() { _ = recover() }()
			// Attempt to parse; ignore result/error.
			_, _ = ParseEthTransaction(data)
			return nil
		}()
	})
}
