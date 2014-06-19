package command

import (
	"fmt"

	"github.com/mitchellh/cli"
)

// This simply patches cli.BasicUi so that it doesn't write an ending
// newline all the time. Also fixes naming convention.
type BasicUI struct {
	cli.BasicUi
}

func (u *BasicUI) Output(message string) {
	fmt.Fprint(u.BasicUi.Writer, message)
}
