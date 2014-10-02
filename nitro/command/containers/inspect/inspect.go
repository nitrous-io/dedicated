package inspect

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/nitrous-io/dedicated/nitro/command/containers"
)

const usage = `
Usage: nitro inspect <container slug>

  inspect a container
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("inspect", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Output(c.Help())
		return 1
	}

	containerStr := args[0]

	c.UI.Output(fmt.Sprintf("%s:", containerStr))
	container, err := c.inspectContainer(containerStr)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	t := PrettyPrint([]string{"Slug", "Image", "State"}, container)
	c.UI.Output(t)
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Inspect a container"
}

func (c *Command) inspectContainer(container string) (*Container, error) {
	url := fmt.Sprintf("%s/api/v0/containers/%s", c.Server, container)
	resp, err := c.Client.GetAuthed(url, nil)
	if err != nil {
		return nil, err
	}
	cnr, err := ParseContainerResponse(resp)
	if err != nil {
		return nil, err
	}
	return cnr, nil
}
