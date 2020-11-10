package blockstore

import (
	"sync"
	"testing"
)

func BenchmarkCast(b *testing.B) {
	temp := Blockstore(NewTemporary())
	for i := 0; i < b.N; i++ {
		if v, ok := temp.(Viewer); ok {
			_ = v
		}
	}
}

func BenchmarkOnce(b *testing.B) {
	var viewer Viewer
	var once sync.Once
	temp := Blockstore(NewTemporary())
	for i := 0; i < b.N; i++ {
		once.Do(func() {
			if v, ok := temp.(Viewer); ok {
				viewer = v
			}
		})
		_ = viewer
	}
}
