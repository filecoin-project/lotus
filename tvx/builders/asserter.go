package builders

import (
	"fmt"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/state"
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
		err := predicate(m.Result)
		a.NoError(err, "message result predicate failed on message %d: %s", i, err)
	}
}

// EveryMessageSenderSatisfies verifies that the sender actors of the supplied
// messages match a condition.
//
// This function groups ApplicableMessages by sender actor, and calls the
// predicate for each unique sender, passing in the initial state (when
// preconditions were committed), the final state (could be nil), and the
// ApplicableMessages themselves.
func (a *Asserter) MessageSendersSatisfy(predicate ActorPredicate, ams ...*ApplicableMessage) {
	bysender := make(map[AddressHandle][]*ApplicableMessage, len(ams))
	for _, am := range ams {
		h := a.b.Actors.HandleFor(am.Message.From)
		bysender[h] = append(bysender[h], am)
	}
	// we now have messages organized by unique senders.
	for sender, amss := range bysender {
		// get precondition state
		pretree, err := state.LoadStateTree(a.b.Stores.CBORStore, a.b.PreRoot)
		a.NoError(err)
		prestate, err := pretree.GetActor(sender.Robust)
		a.NoError(err)

		// get postcondition state; if actor has been deleted, we store a nil.
		poststate, _ := a.b.StateTree.GetActor(sender.Robust)

		// invoke predicate.
		err = predicate(sender, prestate, poststate, amss)
		a.NoError(err, "'every sender actor' predicate failed for sender %s: %s", sender, err)
	}
}

// EveryMessageSenderSatisfies is sugar for MessageSendersSatisfy(predicate, Messages.All()),
// but supports an exclusion set to restrict the messages that will actually be asserted.
func (a *Asserter) EveryMessageSenderSatisfies(predicate ActorPredicate, except ...*ApplicableMessage) {
	ams := a.b.Messages.All()
	if len(except) > 0 {
		filtered := ams[:0]
		for _, ex := range except {
			for _, am := range ams {
				if am == ex {
					continue
				}
				filtered = append(filtered, am)
			}
		}
		ams = filtered
	}
	a.MessageSendersSatisfy(predicate, ams...)
}

func (a *Asserter) FailNow() {
	os.Exit(1)
}

func (a *Asserter) Errorf(format string, args ...interface{}) {
	fmt.Printf("%s: "+format, append([]interface{}{a.stage}, args...))
}
