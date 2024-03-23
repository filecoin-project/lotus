//go:build !windows
// +build !windows

package fsutil

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/xerrors"
)

func Statfs(path string) (FsStat, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return FsStat{}, xerrors.Errorf("statfs: %w", err)
	}

	used := int64(stat.Blocks-stat.Bavail) * int64(stat.Bsize)
	available := int64(stat.Bavail) * int64(stat.Bsize)

	// Filesystems like ZFS can contain sub-volumes which are distinct filesystems, but use the same pool of space.
	// To accurately account for space used in sub-volumes, accumulate the used space in each filesystem mounted under the path.
	mounts, err := getMountsUnder(path)
	if err != nil {
		log.Warnf("could not get all mounts under %s: %e", path, err)
	}

	for _, mount := range mounts {
		var mountStat syscall.Statfs_t
		if err := syscall.Statfs(mount, &mountStat); err != nil {
			log.Warnf("could not get stats for mount %s: %e", mount, err)
			continue
		}
		used += int64(mountStat.Blocks-mountStat.Bavail) * int64(mountStat.Bsize)
	}

	// force int64 to handle platform specific differences
	//nolint:unconvert
	return FsStat{
		Capacity: used + available,

		Available:   available,
		FSAvailable: available,
	}, nil
}

func getMountsUnder(path string) ([]string, error) {
	mounts := []string{}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return mounts, err
	}
	defer f.Close() //nolint

	pathSep := string(os.PathSeparator)
	pathPrefix := path
	if !strings.HasSuffix(path, pathSep) {
		pathPrefix = pathPrefix + pathSep
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := bytes.Fields(scanner.Bytes())
		if len(fields) < 4 {
			continue
		}
		mount, err := unescape(string(fields[4]))
		if err != nil {
			return mounts, err
		}
		if strings.HasPrefix(mount, pathPrefix) {
			mounts = append(mounts, mount)
		}
	}

	return mounts, nil
}

// sourced from https://github.com/moby/sys/blob/1bf36f7188c31f2797c556c4c4d7d9af7a6a965b/mountinfo/mountinfo_linux.go#L158
func unescape(path string) (string, error) {
	// try to avoid copying
	if strings.IndexByte(path, '\\') == -1 {
		return path, nil
	}

	// The following code is UTF-8 transparent as it only looks for some
	// specific characters (backslash and 0..7) with values < utf8.RuneSelf,
	// and everything else is passed through as is.
	buf := make([]byte, len(path))
	bufLen := 0
	for i := 0; i < len(path); i++ {
		if path[i] != '\\' {
			buf[bufLen] = path[i]
			bufLen++
			continue
		}
		s := path[i:]
		if len(s) < 4 {
			// too short
			return "", fmt.Errorf("bad escape sequence %q: too short", s)
		}
		c := s[1]
		switch c {
		case '0', '1', '2', '3', '4', '5', '6', '7':
			v := c - '0'
			for j := 2; j < 4; j++ { // one digit already; two more
				if s[j] < '0' || s[j] > '7' {
					return "", fmt.Errorf("bad escape sequence %q: not a digit", s[:3])
				}
				x := s[j] - '0'
				v = (v << 3) | x
			}
			if v > 255 {
				return "", fmt.Errorf("bad escape sequence %q: out of range" + s[:3])
			}
			buf[bufLen] = v
			bufLen++
			i += 3
			continue
		default:
			return "", fmt.Errorf("bad escape sequence %q: not a digit" + s[:3])

		}
	}

	return string(buf[:bufLen]), nil
}
