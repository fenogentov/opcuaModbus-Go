package modbus

import "testing"

var NameFuncMB = []struct {
	in     string
	expect uint8
	msgErr string
}{
	{"coil", 1, "function does not work "},
	{"discrete", 2, "function does not work "},
	{"holding", 3, "function does not work "},
	{"input", 4, "function does not work "},
	{"coil  ", 1, "extra spaces are not processed"},
	{"  coil", 1, "extra spaces are not processed"},
	{"  coil  ", 1, "extra spaces are not processed"},
	{"COIL", 1, "lowercase letters are not processed "},
	{"registers", 0, "function does not work "},
}

func TestStringToUint8(t *testing.T) {
	for _, el := range NameFuncMB {
		out := StringToUint8(el.in)
		if out != el.expect {
			t.Errorf("%v. Input: \"%v\", expected: %v, received: %v", el.msgErr, el.in, el.expect, out)
		}
	}

}
