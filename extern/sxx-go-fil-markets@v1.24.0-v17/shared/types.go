package shared

// TipSetToken is the implementation-nonspecific identity for a tipset.
type TipSetToken []byte

// Unsubscribe is a function that gets called to unsubscribe from (storage|retrieval)market events
type Unsubscribe func()
