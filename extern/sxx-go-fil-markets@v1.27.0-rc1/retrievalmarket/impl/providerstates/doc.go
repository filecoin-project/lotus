/*
Package providerstates contains state machine logic relating to the `RetrievalProvider`.

provider_fsm.go is where the state transitions are defined, and the default handlers for each new state are defined.

provider_states.go contains state handler functions.

The following diagram illustrates the operation of the provider state machine. This diagram is auto-generated from current code and should remain up to date over time:

https://raw.githubusercontent.com/filecoin-project/go-fil-markets/master/docs/retrievalprovider.mmd.svg

*/
package providerstates
