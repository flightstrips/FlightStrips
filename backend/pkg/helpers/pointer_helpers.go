package helpers

func ValueOrDefault[T any](ptr *T) T {
	if ptr != nil {
		return *ptr
	}
	var zero T
	return zero
}
