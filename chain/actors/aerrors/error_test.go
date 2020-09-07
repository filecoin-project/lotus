package aerrors_test

import (
	"testing"

	"github.com/filecoin-project/go-state-types/exitcode"
	. "github.com/filecoin-project/lotus/chain/actors/aerrors"

	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
)

func TestFatalError(t *testing.T) {
	e1 := xerrors.New("out of disk space")
	e2 := xerrors.Errorf("could not put node: %w", e1)
	e3 := xerrors.Errorf("could not save head: %w", e2)
	ae := Escalate(e3, "failed to save the head")
	aw1 := Wrap(ae, "saving head of new miner actor")
	aw2 := Absorb(aw1, 1, "try to absorb fatal error")
	aw3 := Wrap(aw2, "initializing actor")
	aw4 := Wrap(aw3, "creating miner in storage market")
	t.Logf("Verbose error: %+v", aw4)
	t.Logf("Normal error: %v", aw4)
	assert.True(t, IsFatal(aw4), "should be fatal")
}
func TestAbsorbeError(t *testing.T) {
	e1 := xerrors.New("EOF")
	e2 := xerrors.Errorf("could not decode: %w", e1)
	ae := Absorb(e2, 35, "failed to decode CBOR")
	aw1 := Wrap(ae, "saving head of new miner actor")
	aw2 := Wrap(aw1, "initializing actor")
	aw3 := Wrap(aw2, "creating miner in storage market")
	t.Logf("Verbose error: %+v", aw3)
	t.Logf("Normal error: %v", aw3)
	assert.Equal(t, exitcode.ExitCode(35), RetCode(aw3))
}
