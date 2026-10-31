package provider

// names and valuesOf carry a list of the domain through the snapshot and the
// contract, always as a fresh non-nil slice, so jsonb never stores null.
func names[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

func valuesOf[T ~string](names []string) []T {
	out := make([]T, 0, len(names))
	for _, n := range names {
		out = append(out, T(n))
	}
	return out
}
