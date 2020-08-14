package builders

import (
	"fmt"

	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
)

type ApplyRetPredicate func(ret *vm.ApplyRet) error

func ExitCode(expect exitcode.ExitCode) ApplyRetPredicate {
	return func(ret *vm.ApplyRet) error {
		if ret.ExitCode == expect {
			return nil
		}
		return fmt.Errorf("message exit code was %d; expected %d", ret.ExitCode, expect)
	}
}
