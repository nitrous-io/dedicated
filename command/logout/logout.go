package logout

import (
	"fmt"

	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/auth"
	"github.com/nitrous-io/nitro/command"
	flag "github.com/ogier/pflag"
)

const usage = `
Usage: nitro logout

  logout from authenticated Nitrous.IO account.
`

type Command struct {
	UI     cli.Ui
	Server string
}

func init() {
	command.Register("logout", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(fmt.Sprintf(usage))
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("logout", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	err = auth.RemoveToken(c.Server)
	if err != nil {
		c.UI.Error("Failed to remove the token.")
		return 1
	}

	c.UI.Info("You are no longer logged-in to Nitrous.IO")
	return 0
}

func (c *Command) Synopsis() string {
	return "Logout from authenticated Nitrous.IO account"
}
