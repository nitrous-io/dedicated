package command

import (
	"os"

	"github.com/mitchellh/cli"
)

// Commands is the mapping of all the available Nitro commands.
var Commands map[string]cli.CommandFactory

func init() {
	Commands = map[string]cli.CommandFactory{}
}

var DefaultUI = &BasicUI{cli.BasicUi{
	Reader: os.Stdin,
	Writer: os.Stdout,
}}

func Register(name string, factory cli.CommandFactory) {
	Commands[name] = factory
}
