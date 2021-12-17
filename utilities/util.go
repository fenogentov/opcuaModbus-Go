package utilities

func SetBit(b *byte, i int, val bool) {
	if val {
		*b |= 1 << i
		return
	}
	*b &= ^(1 << i)
}

func FindFromSliceString(sl []string, e string) bool {
	for _, s := range sl {
		if s == e {
			return true
		}
	}
	return false
}
