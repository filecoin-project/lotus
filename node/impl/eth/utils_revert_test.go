package eth

import (
	"encoding/hex"
	"testing"

	ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func abiEncodeErrorString(msg string) []byte {
	// selector 0x08c379a0 + offset (32) + length + bytes padded to 32
	selector, _ := hex.DecodeString("08c379a0")
	// offset = 32 (0x20)
	off := make([]byte, 32)
	off[31] = 32
	// length
	l := make([]byte, 32)
	l[31] = byte(len(msg))
	// data padded
	data := make([]byte, ((len(msg)+31)/32)*32)
	copy(data, []byte(msg))
	// final = selector || off || len || data
	out := append([]byte{}, selector...)
	out = append(out, off...)
	out = append(out, l...)
	out = append(out, data...)
	return out
}

func abiEncodePanic(code uint64) []byte {
	// selector 0x4e487b71 + uint256 code (big-endian)
	selector, _ := hex.DecodeString("4e487b71")
	buf := make([]byte, 32)
	// place code in the last byte; sufficient for known small codes
	buf[31] = byte(code)
	return append(selector, buf...)
}

func TestParseEthRevert_ErrorString(t *testing.T) {
	b := abiEncodeErrorString("oops")
	got := ethtypes.ParseEthRevert(b)
	if got != "Error(oops)" {
		t.Fatalf("unexpected parse: %s", got)
	}
}

func TestParseEthRevert_PanicCodeKnown(t *testing.T) {
	b := abiEncodePanic(0x01) // Assert()
	got := ethtypes.ParseEthRevert(b)
	if got != "Assert()" {
		t.Fatalf("unexpected panic parse: %s", got)
	}
}

func TestParseEthRevert_ShortBuffer(t *testing.T) {
	b := []byte{0x01, 0x02, 0x03}
	got := ethtypes.ParseEthRevert(b)
	// Falls back to hex string of bytes
	if got != ethtypes.EthBytes(b).String() {
		t.Fatalf("unexpected short parse: %s", got)
	}
}
