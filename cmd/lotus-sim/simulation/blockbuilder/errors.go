package blockbuilder

import (
	"errors"
	"fmt"
)

// ErrOutOfGas is returned from BlockBuilder.PushMessage when the block does not have enough gas to
// fit the given message.
type ErrOutOfGas struct {
	Available, Required int64
}

func (e *ErrOutOfGas) Error() string {
	if e.Available == 0 {
		return "out of gas: block full"
	}
	return fmt.Sprintf("out of gas: %d < %d", e.Required, e.Available)
}

// IsOutOfGas returns true if the error is an "out of gas" error.
func IsOutOfGas(err error) bool {
	var oog *ErrOutOfGas
	return errors.As(err, &oog)
}
