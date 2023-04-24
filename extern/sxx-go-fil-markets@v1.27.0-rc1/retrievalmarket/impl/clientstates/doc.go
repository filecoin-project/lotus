/*
Package clientstates contains state machine logic relating to the `RetrievalClient`.

client_fsm.go is where the state transitions are defined, and the default handlers for each new state are defined.

client_states.go contains state handler functions.

The following diagram illustrates the operation of the client state machine. This diagram is auto-generated from current code and should remain up to date over time:

https://raw.githubusercontent.com/filecoin-project/go-fil-markets/master/docs/retrievalclient.mmd.svg

*/
package clientstates
