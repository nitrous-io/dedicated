package start

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const usage = `
Usage: nitro start <slug>

  start a host
`

type Command struct {
	UI     command.UI
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("hosts:start", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	if len(args) != 1 {
		c.UI.Output(c.Help())
		return 1
	}

	slug := args[0]

	c.UI.Output(fmt.Sprintf("Starting host %s...", slug))
	err := c.start(slug)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed: %s", err.Error()))
		return 1
	}

	c.UI.Output("Done.")
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Start a host"
}

func (c *Command) start(slug string) error {
	url := fmt.Sprintf("%s/api/v0/hosts/%s/start", c.Server, slug)
	resp, err := c.Client.PostFormAuthed(url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return httpclient.CheckResponse(resp, http.StatusOK)
}
