package fsutil

import (
	"fmt"
)

type SizeInfo struct {
	OnDisk int64
}

// FileSize returns bytes used by a file or directory on disk
// NOTE: We care about the allocated bytes, not file or directory size
// This is not currently supported on Windows, but at least other lotus can components can build on Windows now
func FileSize(path string) (SizeInfo, error) {
	return SizeInfo{0}, fmt.Errorf("unsupported")
}
