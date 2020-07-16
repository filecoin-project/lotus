package fsutil

type FsStat struct {
	Capacity  int64
	Available int64 // Available to use for sector storage
	Reserved  int64
}
