package clear

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mitchellh/cli"

	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const (
	Usage = `
Usage: nitro keys:clear

  remove all public keys
`
	Warning = "This will REMOVE all keys on your system.  Please type 'YES' without quotes to confirm: "
)

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("keys:clear", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(Usage)
}

func (c *Command) Run(args []string) int {
	y, err := c.UI.Ask(Warning)
	if err != nil {
		if err.Error() == "interrupted" {
			c.UI.Error("keys:clear cancelled")
			return 130
		}
		c.UI.Error(fmt.Sprintf("An unexpected error has occurred: %v", err))
		return 1
	}
	if y != "YES" {
		c.UI.Output("Confirmation not given, quitting\n")
		return 0
	}

	err = c.makeRequest()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	c.UI.Output("All keys removed\n")
	return 0
}

func (c *Command) makeRequest() error {
	resp, err := c.Client.DeleteAuthed(fmt.Sprintf("%s/api/v0/%s", c.Server, "keys"), nil)
	if err != nil {
		return err
	}
	return command.CheckResponse(resp, http.StatusOK)
}

func (c *Command) Synopsis() string {
	return "Remove all public keys"
}
