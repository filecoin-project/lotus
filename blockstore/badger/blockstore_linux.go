package badgerbs

import (
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

func init() {
	fadvWriter = func(fd uintptr) error {
		for _, adv := range []int{unix.FADV_NOREUSE, unix.FADV_DONTNEED} {
			if err := unix.Fadvise(int(fd), 0, 0, adv); err != nil {
				return xerrors.Errorf("fadvise %d failed: %w", adv, err)
			}
		}
		return nil
	}
}
