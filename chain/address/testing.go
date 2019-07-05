package address

import (
	"fmt"
)

// NewForTestGetter returns a closure that returns an address unique to that invocation.
// The address is unique wrt the closure returned, not globally.
func NewForTestGetter() func() Address {
	i := 0
	return func() Address {
		s := fmt.Sprintf("address%d", i)
		i++
		newAddr, err := NewActorAddress([]byte(s))
		if err != nil {
			panic(err)
		}
		return newAddr
	}
}