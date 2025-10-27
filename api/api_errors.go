package api

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var invalidExecutionRevertedMsg = xerrors.New("invalid execution reverted error")

const (
	EOutOfGas = iota + jsonrpc.FirstUserCode
	EActorNotFound
	EF3Disabled
	EF3ParticipationTicketInvalid
	EF3ParticipationTicketExpired
	EF3ParticipationIssuerMismatch
	EF3ParticipationTooManyInstances
	EF3ParticipationTicketStartBeforeExisting
	EF3NotReady
	EExecutionReverted
	ENullRound
	EPaymentChannelDisabled
)

var (
	RPCErrors = jsonrpc.NewErrors()

	// ErrF3Disabled signals that F3 consensus process is disabled.
	ErrF3Disabled = &errF3Disabled{}
	// ErrF3ParticipationTicketInvalid signals that F3ParticipationTicket cannot be decoded.
	ErrF3ParticipationTicketInvalid = &errF3ParticipationTicketInvalid{}
	// ErrF3ParticipationTicketExpired signals that the current GPBFT instance has surpassed the expiry of the ticket.
	ErrF3ParticipationTicketExpired = &errF3ParticipationTicketExpired{}
	// ErrF3ParticipationIssuerMismatch signals that the ticket is not issued by the current node.
	ErrF3ParticipationIssuerMismatch = &errF3ParticipationIssuerMismatch{}
	// ErrF3ParticipationTooManyInstances signals that participation ticket cannot be
	// issued because it asks for too many instances.
	ErrF3ParticipationTooManyInstances = &errF3ParticipationTooManyInstances{}
	// ErrF3ParticipationTicketStartBeforeExisting signals that participation ticket
	// is before the start instance of an existing lease held by the miner.
	ErrF3ParticipationTicketStartBeforeExisting = &errF3ParticipationTicketStartBeforeExisting{}
	// ErrF3NotReady signals that the F3 instance isn't ready for participation yet. The caller
	// should back off and try again later.
	ErrF3NotReady = &errF3NotReady{}
	// ErrPaymentChannelDisabled signals that payment channel operations are disabled.
	ErrPaymentChannelDisabled = &errPaymentChannelDisabled{}

	_ error                 = (*ErrOutOfGas)(nil)
	_ error                 = (*ErrActorNotFound)(nil)
	_ error                 = (*errF3Disabled)(nil)
	_ error                 = (*errF3ParticipationTicketInvalid)(nil)
	_ error                 = (*errF3ParticipationTicketExpired)(nil)
	_ error                 = (*errF3ParticipationIssuerMismatch)(nil)
	_ error                 = (*errF3NotReady)(nil)
	_ error                 = (*ErrExecutionReverted)(nil)
	_ jsonrpc.RPCErrorCodec = (*ErrExecutionReverted)(nil)
	_ error                 = (*ErrNullRound)(nil)
	_ jsonrpc.RPCErrorCodec = (*ErrNullRound)(nil)
	_ error                 = (*errPaymentChannelDisabled)(nil)
)

func init() {
	RPCErrors.Register(EOutOfGas, new(*ErrOutOfGas))
	RPCErrors.Register(EActorNotFound, new(*ErrActorNotFound))
	RPCErrors.Register(EF3Disabled, new(*errF3Disabled))
	RPCErrors.Register(EF3ParticipationTicketInvalid, new(*errF3ParticipationTicketInvalid))
	RPCErrors.Register(EF3ParticipationTicketExpired, new(*errF3ParticipationTicketExpired))
	RPCErrors.Register(EF3ParticipationIssuerMismatch, new(*errF3ParticipationIssuerMismatch))
	RPCErrors.Register(EF3ParticipationTooManyInstances, new(*errF3ParticipationTooManyInstances))
	RPCErrors.Register(EF3ParticipationTicketStartBeforeExisting, new(*errF3ParticipationTicketStartBeforeExisting))
	RPCErrors.Register(EF3NotReady, new(*errF3NotReady))
	RPCErrors.Register(EExecutionReverted, new(*ErrExecutionReverted))
	RPCErrors.Register(ENullRound, new(*ErrNullRound))
	RPCErrors.Register(EPaymentChannelDisabled, new(*errPaymentChannelDisabled))
}

func ErrorIsIn(err error, errorTypes []error) bool {
	for _, etype := range errorTypes {
		tmp := reflect.New(reflect.PointerTo(reflect.ValueOf(etype).Elem().Type())).Interface()
		if errors.As(err, tmp) {
			return true
		}
	}
	return false
}

// ErrOutOfGas signals that a call failed due to insufficient gas.
type ErrOutOfGas struct{}

func (ErrOutOfGas) Error() string { return "call ran out of gas" }

// ErrActorNotFound signals that the actor is not found.
type ErrActorNotFound struct{}

func (ErrActorNotFound) Error() string { return "actor not found" }

type errF3Disabled struct{}

func (errF3Disabled) Error() string { return "f3 is disabled" }

type errF3ParticipationTicketInvalid struct{}

func (errF3ParticipationTicketInvalid) Error() string { return "ticket is not valid" }

type errF3ParticipationTicketExpired struct{}

func (errF3ParticipationTicketExpired) Error() string { return "ticket has expired" }

type errF3ParticipationIssuerMismatch struct{}

func (errF3ParticipationIssuerMismatch) Error() string { return "issuer does not match current node" }

type errF3ParticipationTooManyInstances struct{}

func (errF3ParticipationTooManyInstances) Error() string { return "requested instance count too high" }

type errF3ParticipationTicketStartBeforeExisting struct{}

func (errF3ParticipationTicketStartBeforeExisting) Error() string {
	return "ticket starts before existing lease"
}

type errF3NotReady struct{}

func (errF3NotReady) Error() string { return "f3 isn't yet ready to participate" }

// ErrExecutionReverted is used to return execution reverted with a reason for a revert in the `data` field.
type ErrExecutionReverted struct {
	Message string
	Data    string
}

// Error returns the error message.
func (e *ErrExecutionReverted) Error() string { return e.Message }

// FromJSONRPCError converts a JSONRPCError to ErrExecutionReverted.
func (e *ErrExecutionReverted) FromJSONRPCError(jerr jsonrpc.JSONRPCError) error {
	if jerr.Code != EExecutionReverted || jerr.Message == "" || jerr.Data == nil {
		return invalidExecutionRevertedMsg
	}

	data, ok := jerr.Data.(string)
	if !ok {
		return xerrors.Errorf("expected string data in execution reverted error, got %T", jerr.Data)
	}

	e.Message = jerr.Message
	e.Data = data
	return nil
}

// ToJSONRPCError converts ErrExecutionReverted to a JSONRPCError.
func (e *ErrExecutionReverted) ToJSONRPCError() (jsonrpc.JSONRPCError, error) {
	return jsonrpc.JSONRPCError{
		Code:    EExecutionReverted,
		Message: e.Message,
		Data:    e.Data,
	}, nil
}

// NewErrExecutionReverted creates a new ErrExecutionReverted with the given reason.
func NewErrExecutionReverted(exitCode exitcode.ExitCode, error, reason string, data []byte) *ErrExecutionReverted {
	revertReason := ""
	if reason != "" {
		revertReason = fmt.Sprintf(", revert reason=[%s]", reason)
	}
	return &ErrExecutionReverted{
		Message: fmt.Sprintf("message execution failed (exit=[%s]%s, vm error=[%s])", exitCode, revertReason, error),
		Data:    fmt.Sprintf("0x%x", data),
	}
}

// NewErrExecutionRevertedFromResult creates an ErrExecutionReverted from an InvocResult.
// It decodes the CBOR-encoded return data and parses any Ethereum revert reason.
func NewErrExecutionRevertedFromResult(res *InvocResult) error {
	reason := ""
	var cbytes abi.CborBytes
	if err := cbytes.UnmarshalCBOR(bytes.NewReader(res.MsgRct.Return)); err == nil {
		if len(cbytes) > 0 {
			reason = ethtypes.ParseEthRevert(cbytes)
		} else {
			reason = "none"
		}
	} // else likely a non-ethereum error
	return NewErrExecutionReverted(res.MsgRct.ExitCode, res.Error, reason, cbytes)
}

type ErrNullRound struct {
	Epoch   abi.ChainEpoch
	Message string
}

func NewErrNullRound(epoch abi.ChainEpoch) *ErrNullRound {
	return &ErrNullRound{
		Epoch:   epoch,
		Message: fmt.Sprintf("requested epoch was a null round (%d)", epoch),
	}
}

func (e *ErrNullRound) Error() string {
	return e.Message
}

func (e *ErrNullRound) FromJSONRPCError(jerr jsonrpc.JSONRPCError) error {
	if jerr.Code != ENullRound {
		return fmt.Errorf("unexpected error code: %d", jerr.Code)
	}

	epoch, ok := jerr.Data.(float64)
	if !ok {
		return fmt.Errorf("expected number data in null round error, got %T", jerr.Data)
	}

	e.Epoch = abi.ChainEpoch(epoch)
	e.Message = jerr.Message
	return nil
}

func (e *ErrNullRound) ToJSONRPCError() (jsonrpc.JSONRPCError, error) {
	return jsonrpc.JSONRPCError{
		Code:    ENullRound,
		Message: e.Message,
		Data:    e.Epoch,
	}, nil
}

// Is performs a non-strict type check, we only care if the target is an ErrNullRound
// and will ignore the contents (specifically there is no matching on Epoch).
func (e *ErrNullRound) Is(target error) bool {
	_, ok := target.(*ErrNullRound)
	return ok
}

type errPaymentChannelDisabled struct{}

func (errPaymentChannelDisabled) Error() string {
	return "payment channels disabled (EnablePaymentChannelManager=false)"
}
