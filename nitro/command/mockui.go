package command

import (
	"strings"

	"github.com/mitchellh/cli"
)

type MockUI struct {
	*cli.MockUi
}

func (u *MockUI) Confirm(query string) Confirmation {
	answer, err := u.Ask(query)
	if err != nil {
		return ConfirmErr
	}
	if strings.ToLower(answer) != "y" {
		return ConfirmNo
	}
	return ConfirmYes
}
