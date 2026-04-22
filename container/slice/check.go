package slice

// HasDuplicates returns true if the slice contains duplicates.
func HasDuplicates[T comparable](slice ...T) bool {
	dup := make(map[T]struct{}, len(slice))
	for _, s := range slice {
		if _, ok := dup[s]; ok {
			return true
		}
		dup[s] = struct{}{}
	}
	return false
}
