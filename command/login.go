package command

import (
	"fmt"
	"strings"

	"github.com/howeyc/gopass"
	"github.com/mitchellh/cli"
	flag "github.com/ogier/pflag"
)

var (
	usernameUsage string = "Nitrous.IO username"
	passwordUsage string = "Nitrous.IO password"
)

type LoginCommand struct {
	Ui cli.Ui
}

func (c *LoginCommand) Help() string {
	text := `
Usage: nitro login [options]

  login authenticates with your Nitrous.IO account.

Options:

  -u, --user=""    %s.
  -p, --pass=""    %s.
`
	return strings.TrimSpace(fmt.Sprintf(text, usernameUsage, passwordUsage))
}

func (c *LoginCommand) Run(args []string) int {
	var username string
	var password string

	cmdFlags := flag.NewFlagSet("login", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }
	cmdFlags.StringVarP(&username, "user", "u", "", usernameUsage)
	cmdFlags.StringVarP(&password, "pass", "p", "", passwordUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	if username == "" {
		username, err = c.Ui.Ask("Username:")
		if err != nil {
			c.Ui.Error(fmt.Sprintf("An unexpected error has occurred: %v", err))
		}
	}
	if password == "" {
		c.Ui.Output("Password (will not be echoed): ")
		p := gopass.GetPasswd()
		password = string(p)
	}
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)

	c.Ui.Info(fmt.Sprintf("Logging in with '%v', '%v'", username, password))
	// TODO Actually log in.
	c.Ui.Info(fmt.Sprintf("Logged in as '%v'", username))

	return 0
}

func (c *LoginCommand) Synopsis() string {
	return "Login to Nitrous.IO"
}
