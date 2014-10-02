package command

import (
	"fmt"
	"strings"
)

// a Multiarg is a flag that stores multiple values
// passed to the same flag.  These values can be passed
// with repeated flag usage, or comma-separated.
// The following are equivalent:
//   --env LANG=C --env TMPDIR=/tmp
//   --env LANG=C,TMPDIR=/tmp
// adapted from http://golang.org/src/pkg/flag/example_test.go
type Multiarg []string

const MultiargUsage = "Can be specified multiple times."

func (m *Multiarg) String() string {
	return fmt.Sprint(*m)
}

func (m *Multiarg) Set(value string) error {
	for _, val := range strings.Split(value, ",") {
		*m = append(*m, val)
	}
	return nil
}
