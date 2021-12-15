//stm: #unit
package repo

import (
	"testing"
)

func TestMemBasic(t *testing.T) {
	repo := NewMemory(nil)
	MEM_001
	basicTest(t, repo)
}
