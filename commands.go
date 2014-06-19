package main

import (
	"os"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
)

// Commands is the mapping of all the available Nitro commands.
var Commands map[string]cli.CommandFactory

func init() {
	ui := &command.PatchedBasicUi{cli.BasicUi{
		Reader: os.Stdin,
		Writer: os.Stdout,
	}}

	Commands = map[string]cli.CommandFactory{
		"login": func() (cli.Command, error) {
			return &command.LoginCommand{
				Ui: ui,
			}, nil
		},
	}
}
