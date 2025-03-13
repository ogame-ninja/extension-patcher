package utils

// Ptr returns a pointer to the given argument
func Ptr[T any](v T) *T {
	return &v
}

// Int returns a pointer to the given int
func Int(v int) *int {
	return Ptr(v)
}
