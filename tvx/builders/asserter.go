package builders

import (
	"fmt"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
)

// Asserter offers useful assertions to verify outcomes at various stages of
// the test vector creation.
type Asserter struct {
	*require.Assertions

	b     *Builder
	stage Stage
}

var _ require.TestingT = &Asserter{}

func newAsserter(b *Builder, stage Stage) *Asserter {
	a := &Asserter{stage: stage, b: b}
	a.Assertions = require.New(a)
	return a
}

// In is assert fluid version of require.Contains. It inverts the argument order,
// such that the admissible set can be supplied through assert variadic argument.
func (a *Asserter) In(v interface{}, set ...interface{}) {
	a.Contains(set, v, "set %v does not contain element %v", set, v)
}

// BalanceEq verifies that the balance of the address equals the expected one.
func (a *Asserter) BalanceEq(addr address.Address, expected abi.TokenAmount) {
	actor, err := a.b.StateTree.GetActor(addr)
	a.NoError(err, "failed to fetch actor %s from state", addr)
	a.Equal(expected, actor.Balance, "balances mismatch for address %s", addr)
}

// NonceEq verifies that the nonce of the actor equals the expected one.
func (a *Asserter) NonceEq(addr address.Address, expected uint64) {
	actor, err := a.b.StateTree.GetActor(addr)
	a.NoError(err, "failed to fetch actor %s from state", addr)
	a.Equal(expected, actor.Nonce, "expected actor %s nonce: %d, got: %d", addr, expected, actor.Nonce)
}

// HeadEq verifies that the head of the actor equals the expected one.
func (a *Asserter) HeadEq(addr address.Address, expected cid.Cid) {
	actor, err := a.b.StateTree.GetActor(addr)
	a.NoError(err, "failed to fetch actor %s from state", addr)
	a.Equal(expected, actor.Head, "expected actor %s head: %v, got: %v", addr, expected, actor.Head)
}

// ActorExists verifies that the actor exists in the state tree.
func (a *Asserter) ActorExists(addr address.Address) {
	_, err := a.b.StateTree.GetActor(addr)
	a.NoError(err, "expected no error while looking up actor %s", addr)
}

// ActorExists verifies that the actor is absent from the state tree.
func (a *Asserter) ActorMissing(addr address.Address) {
	_, err := a.b.StateTree.GetActor(addr)
	a.Error(err, "expected error while looking up actor %s", addr)
}

// EveryMessageResultSatisfies verifies that every message result satisfies the
// provided predicate.
func (a *Asserter) EveryMessageResultSatisfies(predicate ApplyRetPredicate, except ...*ApplicableMessage) {
	exceptm := make(map[*ApplicableMessage]struct{}, len(except))
	for _, am := range except {
		exceptm[am] = struct{}{}
	}
	for i, m := range a.b.Messages.messages {
		if _, ok := exceptm[m]; ok {
			continue
		}
		a.NoError(predicate(m.Result), "message result predicate failed on message %d", i)
	}
}

func (a *Asserter) FailNow() {
	os.Exit(1)
}

func (a *Asserter) Errorf(format string, args ...interface{}) {
	fmt.Printf("%s: "+format, append([]interface{}{a.stage}, args...))
}
