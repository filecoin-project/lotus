package result

import "encoding/json"

// Result is a small wrapper type encapsulating Value/Error tuples, mostly for
// use when sending values across channels
// NOTE: Avoid adding any functionality to this, any "nice" things added here will
// make it more difficult to switch to a more standardised Result-like type when
// one gets into the stdlib, or when we will want to switch to a library providing
// those types.
type Result[T any] struct {
	Value T
	Error error
}

func Ok[T any](value T) Result[T] {
	return Result[T]{
		Value: value,
	}
}

func Err[T any](err error) Result[T] {
	return Result[T]{
		Error: err,
	}
}

func Wrap[T any](value T, err error) Result[T] {
	return Result[T]{
		Value: value,
		Error: err,
	}
}

func (r Result[T]) Unwrap() (T, error) {
	return r.Value, r.Error
}

func (r Result[T]) Assert(noErrFn func(err error, msgAndArgs ...interface{})) T {
	noErrFn(r.Error)

	return r.Value
}

// MarshalJSON implements the json.Marshaler interface, marshalling string error correctly
// this method makes the display in log.Infow nicer
func (r Result[T]) MarshalJSON() ([]byte, error) {
	if r.Error != nil {
		return json.Marshal(map[string]string{"Error": r.Error.Error()})
	}

	return json.Marshal(map[string]interface{}{"Value": r.Value})
}
