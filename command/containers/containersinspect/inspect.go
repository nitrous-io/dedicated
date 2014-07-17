package containersinspect

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/nitrous-io/nitro/command/containers"
)

const usage = `
Usage: nitro inspect <host>/<container name>

  inspect a container

Examples:
  nitro inspect host-1/container-a
  nitro inspect container-a
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("inspect", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	if len(args) != 1 {
		c.UI.Output(c.Help())
		return 1
	}

	host, containerStr, err := SplitHostContainer(args[0])
	if err != nil {
		c.UI.Output(c.Help())
		return 1
	}

	c.UI.Output(fmt.Sprintf("%s/%s:", host, containerStr))
	container, err := c.inspectContainer(host, containerStr)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	command.SaveDefaultHost(host)

	t := PrettyPrint([]string{"Name", "Image", "State"}, container)
	c.UI.Output(t)
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Inspect a container"
}

func (c *Command) inspectContainer(host, container string) (*Container, error) {
	resp, err := c.Client.GetAuthed(fmt.Sprintf("%s/api/v0/hosts/%s/containers/%s", c.Server, host, container), nil)
	if err != nil {
		return nil, err
	}
	cnr, err := ParseContainerResponse(resp)
	if err != nil {
		return nil, err
	}
	return cnr, nil
}
