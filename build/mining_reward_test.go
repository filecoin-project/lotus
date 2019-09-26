package build

import (
	"math/big"
	"testing"
)

func TestEncodeMiningRewardInitial(t *testing.T) {
	i := &big.Int{}
	i.SetString("153856870367821447423", 10)
	t.Logf("%#v", i.Bytes())
}
