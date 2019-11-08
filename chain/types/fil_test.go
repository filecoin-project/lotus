package types

import (
	"fmt"
	"testing"
)


func TestFIL_Format(t *testing.T) {
	testValues := []uint64{
		0, 1, 10, 999, 1000, 99900, 878367868, 10000000000,
	}
	testResults := []string{
		"0 aFIL", "1 aFIL", "10 aFIL", "999 aFIL", "1 fFIL", "99.9 fFIL", "878.367868 pFIL", "10 nFIL",
	}

	for i, v := range testValues {
		bi := FIL(NewInt(v))
		res := fmt.Sprintf("%s", bi)
		if res != testResults[i] {
			t.Fatal(res, testResults[i])
		}
	}
}
