package commitment

import (
	"io"
	"os"
	"path/filepath"
)

const treeDFile = "sc-02-data-tree-d.dat"

// TreeDCommD reads CommD from tree-d
func TreeDCommD(cache string) ([32]byte, error) {
	// Open the tree-d file for reading
	file, err := os.Open(filepath.Join(cache, treeDFile))
	if err != nil {
		return [32]byte{}, err
	}
	defer file.Close() // nolint:errcheck

	// Seek to 32 bytes from the end of the file
	_, err = file.Seek(-32, io.SeekEnd)
	if err != nil {
		return [32]byte{}, err
	}

	// Read the last 32 bytes
	var commD [32]byte
	_, err = file.Read(commD[:])
	if err != nil {
		return [32]byte{}, err
	}

	return commD, nil
}
