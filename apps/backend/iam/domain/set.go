package domain

import "slices"

// normalized is the set an aggregate keeps, sorted and without repetition, in
// a fresh slice the caller no longer holds.
func normalized[T ~string](values []T) []T {
	set := slices.Clone(values)
	slices.Sort(set)
	return slices.Compact(set)
}

func union[T ~string](a, b []T) []T {
	return normalized(append(slices.Clone(a), b...))
}

func without[T ~string](values []T, value T) []T {
	return slices.DeleteFunc(slices.Clone(values), func(v T) bool { return v == value })
}
