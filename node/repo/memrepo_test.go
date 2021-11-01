//stm: #unit
package repo

import (
	"testing"
)

func TestMemBasic(t *testing.T) {
	repo := NewMemory(nil)
	//stm: @REPO_MEM_001
	basicTest(t, repo)
}
