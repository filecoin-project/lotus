package builders

import (
	"fmt"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
)

// ApplyRetPredicate evaluates a given condition against the result of a
// message application.
type ApplyRetPredicate func(ret *vm.ApplyRet) error

// OptionalActor is a marker type to warn that the value can be nil.
type OptionalActor = types.Actor

// ActorPredicate evaluates whether the actor that participates in the provided
// messages satisfies a given condition. The initial state (after preconditions)
// and final state (after applies) are supplied.
type ActorPredicate func(handle AddressHandle, initial *OptionalActor, final *OptionalActor, amss []*ApplicableMessage) error

// ExitCode returns an ApplyRetPredicate that passes if the exit code of the
// message execution matches the argument.
func ExitCode(expect exitcode.ExitCode) ApplyRetPredicate {
	return func(ret *vm.ApplyRet) error {
		if ret.ExitCode == expect {
			return nil
		}
		return fmt.Errorf("message exit code was %d; expected %d", ret.ExitCode, expect)
	}
}

// BalanceUpdated returns a ActorPredicate that checks whether the balance
// of the actor has been deducted the gas cost and the outgoing value transfers,
// and has been increased by the offset (or decreased, if the argument is negative).
func BalanceUpdated(offset abi.TokenAmount) ActorPredicate {
	return func(handle AddressHandle, initial *types.Actor, final *OptionalActor, amss []*ApplicableMessage) error {
		if initial == nil || final == nil {
			return fmt.Errorf("BalanceUpdated predicate expected non-nil state")
		}

		// accumulate all balance deductions: ∑(burnt + premium + transferred value)
		deducted := big.Zero()
		for _, am := range amss {
			d := CalculateDeduction(am)
			deducted = big.Add(deducted, d)
		}

		expected := big.Sub(initial.Balance, deducted)
		expected = big.Add(expected, offset)
		if !final.Balance.Equals(expected) {
			return fmt.Errorf("expected balance %s, was: %s", expected, final.Balance)
		}
		return nil
	}
}

// NonceUpdated returns a ActorPredicate that checks whether the nonce
// of the actor has been updated to the nonce of the last message + 1.
func NonceUpdated() ActorPredicate {
	return func(handle AddressHandle, initial *types.Actor, final *OptionalActor, amss []*ApplicableMessage) error {
		if initial == nil || final == nil {
			return fmt.Errorf("BalanceUpdated predicate expected non-nil state")
		}

		// the nonce should be equal to the nonce of the last message + 1.
		last := amss[len(amss)-1]
		if expected, actual := last.Message.Nonce+1, final.Nonce; expected != actual {
			return fmt.Errorf("for actor: %s: expected nonce %d, got %d", handle, expected, actual)
		}
		return nil
	}
}
