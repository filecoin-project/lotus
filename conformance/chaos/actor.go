package chaos

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/rt"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	runtime2 "github.com/filecoin-project/specs-actors/v2/actors/runtime"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

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
	// CallerValidationBranchNone causes no caller validation to take place.
	CallerValidationBranchNone CallerValidationBranch = iota
	// CallerValidationBranchTwice causes Runtime.ValidateImmediateCallerAcceptAny to be called twice.
	CallerValidationBranchTwice
	// CallerValidationBranchIsAddress causes caller validation against CallerValidationArgs.Addrs.
	CallerValidationBranchIsAddress
	// CallerValidationBranchIsType causes caller validation against CallerValidationArgs.Types.
	CallerValidationBranchIsType
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
	// MethodInspectRuntime is the identifier for the method that returns the
	// current runtime values.
	MethodInspectRuntime
	// MethodCreateState is the identifier for the method that creates the chaos actor's state.
	MethodCreateState
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
		MethodInspectRuntime:      a.InspectRuntime,
		MethodCreateState:         a.CreateState,
	}
}

func (a Actor) Code() cid.Cid     { return ChaosActorCodeCID }
func (a Actor) State() cbor.Er    { return new(State) }
func (a Actor) IsSingleton() bool { return true }

var _ rt.VMActor = Actor{}

// SendArgs are the arguments for the Send method.
type SendArgs struct {
	To     address.Address
	Value  abi.TokenAmount
	Method abi.MethodNum
	Params []byte
}

// SendReturn is the return values for the Send method.
type SendReturn struct {
	Return builtin2.CBORBytes
	Code   exitcode.ExitCode
}

// Send requests for this actor to send a message to an actor with the
// passed parameters.
func (a Actor) Send(rt runtime2.Runtime, args *SendArgs) *SendReturn {
	rt.ValidateImmediateCallerAcceptAny()
	var out builtin2.CBORBytes
	code := rt.Send(
		args.To,
		args.Method,
		builtin2.CBORBytes(args.Params),
		args.Value,
		&out,
	)
	return &SendReturn{
		Return: out,
		Code:   code,
	}
}

// Constructor will panic because the Chaos actor is a singleton.
func (a Actor) Constructor(_ runtime2.Runtime, _ *abi.EmptyValue) *abi.EmptyValue {
	panic("constructor should not be called; the Chaos actor is a singleton actor")
}

// CallerValidationArgs are the arguments to Actor.CallerValidation.
type CallerValidationArgs struct {
	Branch CallerValidationBranch
	Addrs  []address.Address
	Types  []cid.Cid
}

// CallerValidation violates VM call validation constraints.
//
//	CallerValidationBranchNone performs no validation.
//	CallerValidationBranchTwice validates twice.
//	CallerValidationBranchIsAddress validates caller against CallerValidationArgs.Addrs.
//	CallerValidationBranchIsType validates caller against CallerValidationArgs.Types.
func (a Actor) CallerValidation(rt runtime2.Runtime, args *CallerValidationArgs) *abi.EmptyValue {
	switch args.Branch {
	case CallerValidationBranchNone:
	case CallerValidationBranchTwice:
		rt.ValidateImmediateCallerAcceptAny()
		rt.ValidateImmediateCallerAcceptAny()
	case CallerValidationBranchIsAddress:
		rt.ValidateImmediateCallerIs(args.Addrs...)
	case CallerValidationBranchIsType:
		rt.ValidateImmediateCallerType(args.Types...)
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
func (a Actor) CreateActor(rt runtime2.Runtime, args *CreateActorArgs) *abi.EmptyValue {
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

func (a Actor) ResolveAddress(rt runtime2.Runtime, args *address.Address) *ResolveAddressResponse {
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
func (a Actor) DeleteActor(rt runtime2.Runtime, beneficiary *address.Address) *abi.EmptyValue {
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

// CreateState creates the chaos actor's state
func (a Actor) CreateState(rt runtime2.Runtime, _ *abi.EmptyValue) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()
	rt.StateCreate(&State{})

	return nil
}

// MutateState attempts to mutate a state value in the actor.
func (a Actor) MutateState(rt runtime2.Runtime, args *MutateStateArgs) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()
	var st State
	switch args.Branch {
	case MutateInTransaction:
		rt.StateTransaction(&st, func() {
			st.Value = args.Value
		})
	case MutateReadonly:
		rt.StateReadonly(&st)
		st.Value = args.Value
	case MutateAfterTransaction:
		rt.StateTransaction(&st, func() {
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
func (a Actor) AbortWith(rt runtime2.Runtime, args *AbortWithArgs) *abi.EmptyValue {
	if args.Uncontrolled { // uncontrolled abort: directly panic
		panic(args.Message)
	}
	rt.Abortf(args.Code, args.Message)
	return nil
}

// InspectRuntimeReturn is the return value for the Actor.InspectRuntime method.
type InspectRuntimeReturn struct {
	Caller         address.Address
	Receiver       address.Address
	ValueReceived  abi.TokenAmount
	CurrEpoch      abi.ChainEpoch
	CurrentBalance abi.TokenAmount
	State          State
}

// InspectRuntime returns a copy of the serializable values available in the Runtime.
func (a Actor) InspectRuntime(rt runtime2.Runtime, _ *abi.EmptyValue) *InspectRuntimeReturn {
	rt.ValidateImmediateCallerAcceptAny()
	var st State
	rt.StateReadonly(&st)
	return &InspectRuntimeReturn{
		Caller:         rt.Caller(),
		Receiver:       rt.Receiver(),
		ValueReceived:  rt.ValueReceived(),
		CurrEpoch:      rt.CurrEpoch(),
		CurrentBalance: rt.CurrentBalance(),
		State:          st,
	}
}
