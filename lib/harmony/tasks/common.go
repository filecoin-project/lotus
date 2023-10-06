package tasks

func SliceIfFound[T any](slice []T, f func(T) bool) []T {
	ct := 0
	for i, v := range slice {
		if f(v) {
			slice[ct], slice[i] = slice[i], slice[ct]
			ct++
		}
	}
	if ct == 0 {
		return slice
	}
	return slice[:ct]
}
