package build

import (
	"math/big"
	"testing"
)

func TestEncodeMiningRewardInitial(t *testing.T) {
	i := &big.Int{}
	i.SetString("153686558225539943338", 10)
	t.Logf("%#v", i.Bytes())
}
