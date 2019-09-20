package rlepluslazy

import (
	"testing"

	"github.com/filecoin-project/go-lotus/extern/rleplus"
	"github.com/stretchr/testify/assert"
)

func TestDecode(t *testing.T) {
	// Encoding bitvec![LittleEndian; 1, 0, 1, 0, 1, 1, 1, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
	// in the Rust reference implementation gives an encoding of [223, 145, 136, 0] (without version field)
	// The bit vector is equivalent to the integer set { 0, 2, 4, 5, 6, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27 }

	// This is the above reference output with a version header "00" manually added
	referenceEncoding := []byte{124, 71, 34, 2}

	expectedNumbers := []uint64{0, 2, 4, 5, 6, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}

	encoded, _, err := rleplus.Encode(expectedNumbers)
	assert.NoError(t, err)

	// Our encoded bytes are the same as the ref bytes
	assert.Equal(t, len(referenceEncoding), len(encoded))
	for idx, expected := range referenceEncoding {
		assert.Equal(t, expected, encoded[idx])
	}

	rle, err := FromBuf(referenceEncoding)
	assert.NoError(t, err)
	decoded := make([]uint64, 0, len(expectedNumbers))

	it, err := rle.Iterator()
	assert.NoError(t, err)
	for it.HasNext() {
		bit, err := it.Next()
		assert.NoError(t, err)
		decoded = append(decoded, bit)
	}

	// Our decoded integers are the same as expected
	assert.Equal(t, expectedNumbers, decoded)

}
