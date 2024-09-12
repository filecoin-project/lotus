package kit

import (
	"bytes"
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/filecoin-project/lotus/lib/must"
)

// CreateRandomFile creates a random file with the provided seed and the
// provided size.
func CreateRandomFile(t *testing.T, rseed, size int) (path string) {
	if size == 0 {
		size = 1600
	}

	source := io.LimitReader(rand.New(rand.NewSource(int64(rseed))), int64(size))

	file, err := os.CreateTemp(t.TempDir(), "sourcefile.dat")
	require.NoError(t, err)

	n, err := io.Copy(file, source)
	require.NoError(t, err)
	require.EqualValues(t, n, size)

	return file.Name()
}

// AssertFilesEqual compares two files by blake2b hash equality and
// fails the test if unequal.
func AssertFilesEqual(t *testing.T, left, right string) {
	// initialize hashes.
	leftH, rightH := must.One(blake2b.New256(nil)), must.One(blake2b.New256(nil))

	// open files.
	leftF, err := os.Open(left)
	require.NoError(t, err)

	rightF, err := os.Open(right)
	require.NoError(t, err)

	// feed hash functions.
	_, err = io.Copy(leftH, leftF)
	require.NoError(t, err)

	_, err = io.Copy(rightH, rightF)
	require.NoError(t, err)

	// compute digests.
	leftD, rightD := leftH.Sum(nil), rightH.Sum(nil)

	require.True(t, bytes.Equal(leftD, rightD))
}
