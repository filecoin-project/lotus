package chaos

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/specs-actors/support/mock"
	atesting "github.com/filecoin-project/specs-actors/support/testing"
)

func TestSingleton(t *testing.T) {
	receiver := atesting.NewIDAddr(t, 100)
	builder := mock.NewBuilder(context.Background(), receiver)

	rt := builder.Build(t)
	var a Actor

	msg := "constructor should not be called; the Chaos actor is a singleton actor"
	rt.ExpectAssertionFailure(msg, func() {
		rt.Call(a.Constructor, abi.Empty)
	})
	rt.Verify()
}

func TestDeleteActor(t *testing.T) {
	receiver := atesting.NewIDAddr(t, 100)
	beneficiary := atesting.NewIDAddr(t, 101)
	builder := mock.NewBuilder(context.Background(), receiver)

	rt := builder.Build(t)
	var a Actor

	rt.ExpectValidateCallerAny()
	rt.ExpectDeleteActor(beneficiary)
	rt.Call(a.DeleteActor, &beneficiary)
	rt.Verify()
}

func TestMutateStateInTransaction(t *testing.T) {
	receiver := atesting.NewIDAddr(t, 100)
	builder := mock.NewBuilder(context.Background(), receiver)

	rt := builder.Build(t)
	var a Actor

	rt.ExpectValidateCallerAny()
	rt.Create(&State{})

	val := "__mutstat test"
	rt.Call(a.MutateState, &MutateStateArgs{
		Value:  val,
		Branch: MutateInTransaction,
	})

	var st State
	rt.GetState(&st)

	if st.Value != val {
		t.Fatal("state was not updated")
	}

	rt.Verify()
}

func TestMutateStateAfterTransaction(t *testing.T) {
	receiver := atesting.NewIDAddr(t, 100)
	builder := mock.NewBuilder(context.Background(), receiver)

	rt := builder.Build(t)
	var a Actor

	rt.ExpectValidateCallerAny()
	rt.Create(&State{})

	val := "__mutstat test"
	rt.Call(a.MutateState, &MutateStateArgs{
		Value:  val,
		Branch: MutateAfterTransaction,
	})

	var st State
	rt.GetState(&st)

	// state should be updated successfully _in_ the transaction but not outside
	if st.Value != val+"-in" {
		t.Fatal("state was not updated")
	}

	rt.Verify()
}

func TestMutateStateReadonly(t *testing.T) {
	receiver := atesting.NewIDAddr(t, 100)
	builder := mock.NewBuilder(context.Background(), receiver)

	rt := builder.Build(t)
	var a Actor

	rt.ExpectValidateCallerAny()
	rt.Create(&State{})

	val := "__mutstat test"
	rt.Call(a.MutateState, &MutateStateArgs{
		Value:  val,
		Branch: MutateReadonly,
	})

	var st State
	rt.GetState(&st)

	if st.Value != "" {
		t.Fatal("state was not expected to be updated")
	}

	rt.Verify()
}

func TestAbortWith(t *testing.T) {
	receiver := atesting.NewIDAddr(t, 100)
	builder := mock.NewBuilder(context.Background(), receiver)

	rt := builder.Build(t)
	var a Actor

	msg := "__test forbidden"
	rt.ExpectAbortContainsMessage(exitcode.ErrForbidden, msg, func() {
		rt.Call(a.AbortWith, &AbortWithArgs{
			Code:         exitcode.ErrForbidden,
			Message:      msg,
			Uncontrolled: false,
		})
	})
	rt.Verify()
}

func TestAbortWithUncontrolled(t *testing.T) {
	receiver := atesting.NewIDAddr(t, 100)
	builder := mock.NewBuilder(context.Background(), receiver)

	rt := builder.Build(t)
	var a Actor

	msg := "__test uncontrolled panic"
	rt.ExpectAssertionFailure(msg, func() {
		rt.Call(a.AbortWith, &AbortWithArgs{
			Message:      msg,
			Uncontrolled: true,
		})
	})
	rt.Verify()
}
