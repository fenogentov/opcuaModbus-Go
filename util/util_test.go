package util

import "testing"

var tstSetBit = []struct {
	numBit int
	value  bool
	want   byte
}{
	{
		numBit: 0,
		value:  true,
		want:   1,
	},
	{
		numBit: 7,
		value:  true,
		want:   129,
	},
	{
		numBit: 4,
		value:  true,
		want:   145,
	},
	{
		numBit: 7,
		value:  false,
		want:   17,
	},
}

func TestSetBit(t *testing.T) {
	var b byte
	for _, el := range tstSetBit {
		SetBit(&b, el.numBit, el.value)
		if el.want != b {
			t.Errorf("got: %v, wanted: %v", b, el.want)
		}
	}
}

func TestFindFromSliceString(t *testing.T) {
	tstSlice := []string{"string_1", "string_2", "string_3", "string_4", "string_5"}
	if !FindFromSliceString(tstSlice, "string_1") {
		t.Errorf("got: false, wanted: true")
	}
	if FindFromSliceString(tstSlice, "string_6") {
		t.Errorf("got: true, wanted: false")
	}
}
