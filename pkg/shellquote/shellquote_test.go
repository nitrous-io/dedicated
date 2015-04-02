package shellquote

import (
	"fmt"
	"testing"
)

func TestQuoteArg(t *testing.T) {
	tests := [][]string{
		{"a", "a"},
		{"foo ", `"foo "`},
		{"  f  o ", `"  f  o "`},
		{"*", "*"},
	}
	for _, test := range tests {
		exp := test[1]
		got := quoteCmdArg(test[0])
		if exp != got {
			t.Errorf("quoteCmdArg(%q): got %q, expected %q", test[0], got, exp)
		}
	}
}

// https://github.com/python/cpython/blob/master/Lib/test/test_shlex.py#L177
func TestQuoteCmdSafe(t *testing.T) {
	safe := "abcdefghijklmnopqrstuvwxyz" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	safe += "0123456789" + "@%_-+=:,./"
	for i, _ := range safe {
		exp := fmt.Sprintf("test%cname", safe[i])
		got := QuoteCmd(exp)
		if got != exp {
			t.Errorf("got: %q, exp: %q", got, exp)
		}
	}
}

// https://github.com/python/cpython/blob/master/Lib/test/test_shlex.py#L177
func TestQuoteCmdUnsafe(t *testing.T) {
	unsafe := `"$\\!` + "`"
	for i, _ := range unsafe {
		arg := fmt.Sprintf("test%cname", unsafe[i])
		exp := fmt.Sprintf("'test%cname'", unsafe[i])
		got := QuoteCmd(arg)
		if got != exp {
			t.Errorf("\narg: %s\ngot: %s\nexp: %s\n", arg, got, exp)
		}

		arg = fmt.Sprintf("test%c'name'", unsafe[i])
		exp = fmt.Sprintf("'test%c'\"'\"'name'\"'\"''", unsafe[i])
		got = QuoteCmd(arg)
		if got != exp {
			t.Errorf("\narg: %s\ngot: %s\nexp: %s\n", arg, got, exp)
		}
	}
}
