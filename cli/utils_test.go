package cli

import (
	"testing"

	types "github.com/filecoin-project/lotus/chain/types"
	"github.com/stretchr/testify/assert"
)

func TestSizeStr(t *testing.T) {
	cases := []struct {
		in  uint64
		out string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1024, "1 KiB"},
		{2000, "1.95 KiB"},
		{5 << 20, "5 MiB"},
		{11 << 60, "11 EiB"},
	}

	for _, c := range cases {
		assert.Equal(t, c.out, SizeStr(types.NewInt(c.in)), "input %+v, produced wrong result", c)
	}

}
