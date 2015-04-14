package finder

import "testing"

func TestSanitizeFilePattern(t *testing.T) {
	var tests = []string{
		`foo`, `foo`,
		`*.js`, `.*\.js`,
		`.*go`, `.*go`,
		`ma(lo`, `ma\(lo`,
	}
	n := len(tests) / 2
	for i := 0; i < n; i++ {
		test := tests[i*2]
		got := SanitizeFilePattern(test)
		exp := tests[i*2+1]
		if got != exp {
			t.Errorf("for '%s' got '%s', expected '%s'", test, got, exp)
		}
	}
}
