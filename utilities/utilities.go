package utilities

// SetBit is sets the required bit to a value
func SetBit(b *byte, i int, val bool) {
	if val {
		*b |= 1 << i
		return
	}
	*b &= ^(1 << i)
}

// FindFromSliceString is defines the location of a string in a slice
func FindFromSliceString(sl []string, e string) bool {
	for _, s := range sl {
		if s == e {
			return true
		}
	}
	return false
}
