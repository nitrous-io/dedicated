package containersstart

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
Usage: nitro start <host>/<container name>

  start a container

Examples:
  nitro start host-1/container-a
  nitro start container-a
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("start", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("start", flag.ContinueOnError)
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

	c.UI.Output(fmt.Sprintf("Starting %s/%s...", host, containerStr))
	container, err := c.start(host, containerStr)
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
	return "Start a container"
}

func (c *Command) start(host, container string) (*Container, error) {
	url := fmt.Sprintf("%s/api/v0/hosts/%s/containers/%s/start", c.Server, host, container)
	resp, err := c.Client.PostFormAuthed(url, nil)
	if err != nil {
		return nil, err
	}
	cnr, err := ParseContainerResponse(resp)
	if err != nil {
		return nil, err
	}
	return cnr, nil
}
