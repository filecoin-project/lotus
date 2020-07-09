package fsutil

import (
	"syscall"

	"golang.org/x/xerrors"
)

type SizeInfo struct {
	OnDisk int64
}

// FileSize returns bytes used by a file on disk
func FileSize(path string) (SizeInfo, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return SizeInfo{}, xerrors.Errorf("stat: %w", err)
	}

	// NOTE: stat.Blocks is in 512B blocks, NOT in stat.Blksize
	//  See https://www.gnu.org/software/libc/manual/html_node/Attribute-Meanings.html
	return SizeInfo{
		int64(stat.Blocks) * 512,
	}, nil
}
