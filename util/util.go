package util

// SetBit is sets required bit in byte (bits are counted from 0)
func SetBit(b *byte, i int, val bool) {
	if val {
		*b |= 1 << i
		return
	}
	*b &= ^(1 << i)
}

// FindFromSliceString is detects presence string in []string
func FindFromSliceString(sl []string, e string) bool {
	for _, s := range sl {
		if s == e {
			return true
		}
	}
	return false
}
