package ethtypes

const defaultMessage = "execution reverted"

// ExecutionRevertedError is an error that occurs when a transaction reverts.
type ExecutionRevertedError struct {
	Message string
	Code    int
	Data    string
}

// Error implements the error interface.
func (e *ExecutionRevertedError) Error() string {
	return e.Message
}

// ErrorData returns the error data.
func (e *ExecutionRevertedError) ErrorData() interface{} {
	return e.Data
}

// ErrorCode returns the JSON error code for a revert.
func (e *ExecutionRevertedError) ErrorCode() int {
	return e.Code
}

// NewExecutionRevertedErrorWithData returns an ExecutionRevertedError with the given code and data.
func NewExecutionRevertedError(code int, data string) *ExecutionRevertedError {
	return &ExecutionRevertedError{
		Message: defaultMessage,
		Code:    code,
		Data:    data,
	}
}
