package command

import (
	"fmt"

	"github.com/mitchellh/cli"
)

// PatchedBasicUi simply patches cli.BasicUi so that it doesn't write an ending
// newline all the time.
type PatchedBasicUi struct {
	cli.BasicUi
}

func (u *PatchedBasicUi) Output(message string) {
	fmt.Fprint(u.BasicUi.Writer, message)
}
