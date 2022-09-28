package mir

import (
	"fmt"
	"path"

	"github.com/filecoin-project/mir/pkg/simplewal"
)

func NewWAL(ownID, walDir string) (*simplewal.WAL, error) {
	walPath := path.Join(walDir, fmt.Sprintf("%v", ownID))
	wal, err := simplewal.Open(walPath)
	if err != nil {
		return nil, err
	}
	return wal, nil
}
