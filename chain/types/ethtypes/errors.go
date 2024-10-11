package ethtypes

import (
	"fmt"

	"golang.org/x/xerrors"
)

var ErrExecutionReverted = xerrors.New("execution reverted")

// ExecutionRevertedError is an error that occurs when a transaction reverts.
type ExecutionRevertedError struct {
	Message string
	Code    int
	Meta    []byte
	Data    string
}

// Error implements the error interface.
func (e *ExecutionRevertedError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("json-rpc error %d", e.Code)
	}
	return e.Message
}

// ErrorData returns the error data.
func (e *ExecutionRevertedError) ErrorData() interface{} {
	return e.Data
}

// ErrorCode returns the JSON error code for a revert.
func (e *ExecutionRevertedError) ErrorCode() int {
	return 3
}

// NewExecutionRevertedWithDataError returns an ExecutionRevertedError with the given code and data.
func NewExecutionRevertedWithDataError(code int, data string) *ExecutionRevertedError {
	return &ExecutionRevertedError{
		Message: ErrExecutionReverted.Error(),
		Code:    code,
		Data:    data,
	}
}
