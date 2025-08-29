package delegator

// Actor is a placeholder to outline the intended API.
// Future work should replace this with a fully implemented actor.
type Actor struct{}

// ApplyDelegations remains a no-op placeholder; full wiring will provide runtime
// context and authority recovery. Use State.ApplyDelegationsWithAuthorities for
// unit-level validation of mapping writes and nonce handling.
func (Actor) ApplyDelegations(_ []DelegationParam) error { return nil }
