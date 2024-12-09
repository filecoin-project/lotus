package ptr

func PtrTo[T any](t T) *T {
	return &t
}
