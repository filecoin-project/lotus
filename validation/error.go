package validation

import (
	"fmt"

	"golang.org/x/xerrors"
)

// Hierarchy of error categories to programmatically preserve in the language
// information linking errors from different contexts. Used inside
// `ErrorWithCategory`, it complements the standard `xerrors.Errorf` wrapping
// mechanism that preserves call stack information. This is not an error itself,
// it explicitly does not implement the interface forcing explicit calls to
// convert a category into a specific error instance used in validation functions.
// The hierarchy is implemented through a pointer to a (unique) `parent` category
// (if any). The `description` string identifies the category and will be used
// as the error message if one is not explicitly provided (like in `FromString()`).
// FIXME: This is general enough to be outside of this package.
type ErrorCategory struct {
	parent      *ErrorCategory
	description string
}

func NewHierarchicalErrorClass(description string) *ErrorCategory {
	return &ErrorCategory{description: description}
}

// Creates a new (child) category included in this one.
func (c *ErrorCategory) Child(description string) *ErrorCategory {
	return &ErrorCategory{parent: c, description: description}
}

// Returns a new error with the category description as its message.
func (c *ErrorCategory) Error() error {
	return &ErrorWithCategory{category: c, msg: c.description, frame: getCallerFrame()}
}

// Returns a new error from this category with the specified message.
func (c *ErrorCategory) FromString(msg string) error {
	return &ErrorWithCategory{category: c, msg: msg, frame: getCallerFrame()}
}

// Similar to `Error`, returns an error err.
func (c *ErrorCategory) WrapError(wrappedError error) error {
	err := c.Error().(ErrorWithCategory)
	err.wrappedError = wrappedError
	return err
}

// Error implementing the `error` interface providing category information
// and wrapped error semantics similar to `xerrors.Errorf`, including frame
// information (based on `xerrors.wrapError`).
type ErrorWithCategory struct {
	category *ErrorCategory

	// Copied `xerrors.wrapError` attributes (since its a private structure)
	wrappedError error
	msg          string
	frame        xerrors.Frame
}

func (e *ErrorWithCategory) Category() *ErrorCategory {
	return e.category
}

// FIXME: Why can't it have a pointer receiver to satisfy the `error` interface?
//  (`xerrors.wrapError` uses pointer)
func (e ErrorWithCategory) Error() string {
	return fmt.Sprint(e)
}

func (e *ErrorWithCategory) Format(s fmt.State, v rune) { xerrors.FormatError(e, s, v) }

func (e *ErrorWithCategory) FormatError(p xerrors.Printer) (next error) {
	p.Print(e.msg)
	// FIXME: If we're using an external string as the error message we should
	//  probably want to include the category description here.
	e.frame.Format(p)
	return e.wrappedError
}

func (e *ErrorWithCategory) Unwrap() error {
	return e.wrappedError
}

func getCallerFrame() xerrors.Frame {
	frame := xerrors.Frame{}
	// if internal.EnableTrace {
	frame = xerrors.Caller(1)
	// }
	// FIXME: Can't reference internal package, if the flag is necessary we can extract
	//  it from an oracle of `xerrors` behavior (hacky).

	return frame
}
