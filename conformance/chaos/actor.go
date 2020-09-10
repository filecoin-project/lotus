package chaos

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/ipfs/go-cid"

	typegen "github.com/whyrusleeping/cbor-gen"
)

//go:generate go run ./gen

// Actor is a chaos actor. It implements a variety of illegal behaviours that
// trigger violations of VM invariants. These behaviours are not found in
// production code, but are important to test that the VM constraints are
// properly enforced.
//
// The chaos actor is being incubated and its behaviour and ABI be standardised
// shortly. Its CID is ChaosActorCodeCID, and its singleton address is 98 (Address).
// It cannot be instantiated via the init actor, and its constructor panics.
//
// Test vectors relying on the chaos actor being deployed will carry selector
// "chaos_actor:true".
type Actor struct{}

// CallerValidationBranch is an enum used to select a branch in the
// CallerValidation method.
type CallerValidationBranch int64

const (
	CallerValidationBranchNone CallerValidationBranch = iota
	CallerValidationBranchTwice
	CallerValidationBranchAddrNilSet
	CallerValidationBranchTypeNilSet
)

// MutateStateBranch is an enum used to select the type of state mutation to attempt.
type MutateStateBranch int64

const (
	// MutateInTransaction legally mutates state within a transaction.
	MutateInTransaction MutateStateBranch = iota
	// MutateReadonly ILLEGALLY mutates readonly state.
	MutateReadonly
	// MutateAfterTransaction ILLEGALLY mutates state after a transaction.
	MutateAfterTransaction
)

const (
	_                      = 0 // skip zero iota value; first usage of iota gets 1.
	MethodCallerValidation = builtin.MethodConstructor + iota
	MethodCreateActor
	MethodResolveAddress
	// MethodDeleteActor is the identifier for the method that deletes this actor.
	MethodDeleteActor
	// MethodSend is the identifier for the method that sends a message to another actor.
	MethodSend
	// MethodMutateState is the identifier for the method that attempts to mutate
	// a state value in the actor.
	MethodMutateState
	// MethodAbortWith is the identifier for the method that panics optionally with
	// a passed exit code.
	MethodAbortWith
)

// Exports defines the methods this actor exposes publicly.
func (a Actor) Exports() []interface{} {
	return []interface{}{
		builtin.MethodConstructor: a.Constructor,
		MethodCallerValidation:    a.CallerValidation,
		MethodCreateActor:         a.CreateActor,
		MethodResolveAddress:      a.ResolveAddress,
		MethodDeleteActor:         a.DeleteActor,
		MethodSend:                a.Send,
		MethodMutateState:         a.MutateState,
		MethodAbortWith:           a.AbortWith,
	}
}

var _ runtime.Invokee = Actor{}

// SendArgs are the arguments for the Send method.
type SendArgs struct {
	To     address.Address
	Value  abi.TokenAmount
	Method abi.MethodNum
	Params []byte
}

// SendReturn is the return values for the Send method.
type SendReturn struct {
	Return runtime.CBORBytes
	Code   exitcode.ExitCode
}

// Send requests for this actor to send a message to an actor with the
// passed parameters.
func (a Actor) Send(rt runtime.Runtime, args *SendArgs) *SendReturn {
	rt.ValidateImmediateCallerAcceptAny()
	ret, code := rt.Send(
		args.To,
		args.Method,
		runtime.CBORBytes(args.Params),
		args.Value,
	)
	var out runtime.CBORBytes
	if ret != nil {
		if err := ret.Into(&out); err != nil {
			rt.Abortf(exitcode.ErrIllegalState, "failed to unmarshal send return: %v", err)
		}
	}
	return &SendReturn{
		Return: out,
		Code:   code,
	}
}

// Constructor will panic because the Chaos actor is a singleton.
func (a Actor) Constructor(_ runtime.Runtime, _ *abi.EmptyValue) *abi.EmptyValue {
	panic("constructor should not be called; the Chaos actor is a singleton actor")
}

// CallerValidation violates VM call validation constraints.
//
//  CallerValidationBranchNone performs no validation.
//  CallerValidationBranchTwice validates twice.
//  CallerValidationBranchAddrNilSet validates against an empty caller
//  address set.
//  CallerValidationBranchTypeNilSet validates against an empty caller type set.
func (a Actor) CallerValidation(rt runtime.Runtime, branch *typegen.CborInt) *abi.EmptyValue {
	switch CallerValidationBranch(*branch) {
	case CallerValidationBranchNone:
	case CallerValidationBranchTwice:
		rt.ValidateImmediateCallerAcceptAny()
		rt.ValidateImmediateCallerAcceptAny()
	case CallerValidationBranchAddrNilSet:
		rt.ValidateImmediateCallerIs()
	case CallerValidationBranchTypeNilSet:
		rt.ValidateImmediateCallerType()
	default:
		panic("invalid branch passed to CallerValidation")
	}

	return nil
}

// CreateActorArgs are the arguments to CreateActor.
type CreateActorArgs struct {
	// UndefActorCID instructs us to use cid.Undef; we can't pass cid.Undef
	// in ActorCID because it doesn't serialize.
	UndefActorCID bool
	ActorCID      cid.Cid

	// UndefAddress is the same as UndefActorCID but for Address.
	UndefAddress bool
	Address      address.Address
}

// CreateActor creates an actor with the supplied CID and Address.
func (a Actor) CreateActor(rt runtime.Runtime, args *CreateActorArgs) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()

	var (
		acid = args.ActorCID
		addr = args.Address
	)

	if args.UndefActorCID {
		acid = cid.Undef
	}
	if args.UndefAddress {
		addr = address.Undef
	}

	rt.CreateActor(acid, addr)
	return nil
}

// ResolveAddressResponse holds the response of a call to runtime.ResolveAddress
type ResolveAddressResponse struct {
	Address address.Address
	Success bool
}

func (a Actor) ResolveAddress(rt runtime.Runtime, args *address.Address) *ResolveAddressResponse {
	rt.ValidateImmediateCallerAcceptAny()

	resolvedAddr, ok := rt.ResolveAddress(*args)
	if !ok {
		invalidAddr, _ := address.NewIDAddress(0)
		resolvedAddr = invalidAddr
	}
	return &ResolveAddressResponse{resolvedAddr, ok}
}

// DeleteActor deletes the executing actor from the state tree, transferring any
// balance to beneficiary.
func (a Actor) DeleteActor(rt runtime.Runtime, beneficiary *address.Address) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()
	rt.DeleteActor(*beneficiary)
	return nil
}

// MutateStateArgs specify the value to set on the state and the way in which
// it should be attempted to be set.
type MutateStateArgs struct {
	Value  string
	Branch MutateStateBranch
}

// MutateState attempts to mutate a state value in the actor.
func (a Actor) MutateState(rt runtime.Runtime, args *MutateStateArgs) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()
	var st State
	switch args.Branch {
	case MutateInTransaction:
		rt.State().Transaction(&st, func() {
			st.Value = args.Value
		})
	case MutateReadonly:
		rt.State().Readonly(&st)
		st.Value = args.Value
	case MutateAfterTransaction:
		rt.State().Transaction(&st, func() {
			st.Value = args.Value + "-in"
		})
		st.Value = args.Value
	default:
		panic("unknown mutation type")
	}
	return nil
}

// AbortWithArgs are the arguments to the Actor.AbortWith method, specifying the
// exit code to (optionally) abort with and the message.
type AbortWithArgs struct {
	Code         exitcode.ExitCode
	Message      string
	Uncontrolled bool
}

// AbortWith simply causes a panic with the passed exit code.
func (a Actor) AbortWith(rt runtime.Runtime, args *AbortWithArgs) *abi.EmptyValue {
	if args.Uncontrolled { // uncontrolled abort: directly panic
		panic(args.Message)
	} else {
		rt.Abortf(args.Code, args.Message)
	}
	return nil
}
