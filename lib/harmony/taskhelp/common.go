package taskhelp

// SubsetIf returns a subset of the slice for which the predicate is true.
// It does not allocate memory, but rearranges the list in place.
// A non-zero list input will always return a non-zero list.
// The return value is the subset and a boolean indicating whether the subset was sliced.
func SliceIfFound[T any](slice []T, f func(T) bool) ([]T, bool) {
	ct := 0
	for i, v := range slice {
		if f(v) {
			slice[ct], slice[i] = slice[i], slice[ct]
			ct++
		}
	}
	if ct == 0 {
		return slice, false
	}
	return slice[:ct], true
}
