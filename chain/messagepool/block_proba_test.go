package messagepool

import "testing"

func TestBlockProbability(t *testing.T) {
	mp := &MessagePool{}
	t.Logf("%+v\n", mp.blockProbabilities(1-0.15))
}
