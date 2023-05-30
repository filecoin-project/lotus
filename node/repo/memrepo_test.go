// stm: #unit
package repo

import (
	"testing"
)

func TestMemBasic(t *testing.T) {
	//stm: @REPO_MEM_001
	repo := NewMemory(nil)
	basicTest(t, repo)
}
