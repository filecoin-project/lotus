package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeBlockMsg(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    *BlockMsg
		wantErr bool
	}{
		{"decode empty BlockMsg with extra data at the end", []byte{0x83, 0xf6, 0x80, 0x80, 0x20}, nil, true},
		{"decode valid empty BlockMsg", []byte{0x83, 0xf6, 0x80, 0x80}, new(BlockMsg), false},
		{"decode invalid cbor", []byte{0x83, 0xf6, 0x80}, nil, true},
	}
	for _, tt := range tests {
		data := tt.data
		want := tt.want
		wantErr := tt.wantErr
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeBlockMsg(data)
			if wantErr {
				assert.Errorf(t, err, "DecodeBlockMsg(%x)", data)
				return
			}
			assert.NoErrorf(t, err, "DecodeBlockMsg(%x)", data)
			assert.Equalf(t, want, got, "DecodeBlockMsg(%x)", data)
			serialized, err := got.Serialize()
			assert.NoErrorf(t, err, "DecodeBlockMsg(%x)", data)
			assert.Equalf(t, serialized, data, "DecodeBlockMsg(%x)", data)
		})
	}
}
