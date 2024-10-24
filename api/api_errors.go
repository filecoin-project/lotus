package api

import (
	"errors"
	"reflect"

	"github.com/filecoin-project/go-jsonrpc"
)

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
)

var (
	RPCErrors = jsonrpc.NewErrors()

	// ErrF3Disabled signals that F3 consensus process is disabled.
	ErrF3Disabled = &errF3Disabled{}
	// ErrF3ParticipationTicketInvalid signals that F3ParticipationTicket cannot be decoded.
	ErrF3ParticipationTicketInvalid = &errF3ParticipationTicketInvalid{}
	// ErrF3ParticipationTicketExpired signals that the current GPBFT instance as surpassed the expiry of the ticket.
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

	_ error = (*ErrOutOfGas)(nil)
	_ error = (*ErrActorNotFound)(nil)
	_ error = (*errF3Disabled)(nil)
	_ error = (*errF3ParticipationTicketInvalid)(nil)
	_ error = (*errF3ParticipationTicketExpired)(nil)
	_ error = (*errF3ParticipationIssuerMismatch)(nil)
	_ error = (*errF3NotReady)(nil)
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
