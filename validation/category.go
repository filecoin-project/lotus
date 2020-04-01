package validation

// Errors extracted from `ValidateBlock` and `checkBlockMessages` and converted
// into more generic "categories" that are related with each other in a hierarchical
// fashion and allow to spawn errors from them.
// Explicitly split into separate `var` declarations instead of having a single
// block (to avoid `go fmt` potentially changing alignment of all errors and
// producing harder-to-read diffs).
// FIXME: How to better express this error hierarchy *without* reflection? (nested packages?)

var InvalidBlock = NewHierarchicalErrorClass("invalid block")

var NilSignature = InvalidBlock.Child("nil signature")

var Timestamp = InvalidBlock.Child("invalid timestamp")
var BlockFutureTimestamp = Timestamp.Child("ahead of current time")

var BlockMinedEarly = InvalidBlock.Child("block was generated too soon")

var Winner = InvalidBlock.Child("not a winner")
var SlashedMiner = Winner.Child("slashed or invalid miner")
var NoCandidates = Winner.Child("no candidates")
var DuplicateCandidates = Winner.Child("duplicate epost candidates")

// FIXME: Might want to include these in some EPost category.

var Miner = InvalidBlock.Child("invalid miner")

var InvalidMessagesCID = InvalidBlock.Child("messages CID didn't match message root in header")

var InvalidMessage = InvalidBlock.Child("invalid message")
var InvalidBlsSignature = InvalidMessage.Child("invalid bls aggregate signature")
var InvalidSecpkSignature = InvalidMessage.Child("invalid secpk signature")
var EmptyRecipient = InvalidMessage.Child("empty 'To' address")
var WrongNonce = InvalidMessage.Child("wrong nonce")

// FIXME: Errors from `checkBlockMessages` are too generic and should probably be extracted, is there
//  another place where we validate them (like the message pool)? In that case we need a different
//  root category like `InvalidMessage`.
