//go:build darwin || freebsd || openbsd || dragonfly || netbsd
// +build darwin freebsd openbsd dragonfly netbsd

package resources

import (
	"encoding/binary"
	"syscall"
)

func sysctlUint64(name string) (uint64, error) {
	s, err := syscall.Sysctl(name)
	if err != nil {
		return 0, err
	}
	// hack because the string conversion above drops a \0
	b := []byte(s)
	if len(b) < 8 {
		b = append(b, 0)
	}
	return binary.LittleEndian.Uint64(b), nil
}
