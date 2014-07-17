package containersstop

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"

	. "github.com/nitrous-io/nitro/command/containers"
)

const forceUsage = "Skip confirmation"

const usage = `
Usage: nitro stop <host>/<container name>

  stop a container

Options:

  -f, --force=""    %s

Examples:
  nitro stop host-1/container-a
  nitro stop container-a
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("stop", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Run(args []string) int {
	var force bool

	cmdFlags := flag.NewFlagSet("stop", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.BoolVarP(&force, "force", "f", false, forceUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	if len(args) < 1 {
		c.UI.Output(c.Help())
		return 1
	}

	host, containerStr, err := SplitHostContainer(args[0])
	if err != nil {
		c.UI.Output(c.Help())
		return 1
	}

	if !force {
		answer, err := c.UI.Ask(fmt.Sprintf("Do you really want to stop '%s/%s'? [y/N]", host, containerStr))
		if err != nil {
			if err.Error() == "interrupted" {
				c.UI.Error("Cancelled.")
				return 130
			}
			c.UI.Error(fmt.Sprintf("An unexpected error has occurred: %v", err))
			return 1
		}

		if strings.ToLower(answer) != "y" {
			c.UI.Output("Cancelled.")
			return 0
		}
	}

	c.UI.Output(fmt.Sprintf("Stopping %s/%s...", host, containerStr))
	container, err := c.stop(host, containerStr)
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
	return strings.TrimSpace(fmt.Sprintf(usage, forceUsage))
}

func (c *Command) Synopsis() string {
	return "Stop a container"
}

func (c *Command) stop(host, container string) (*Container, error) {
	url := fmt.Sprintf("%s/api/v0/hosts/%s/containers/%s/stop", c.Server, host, container)
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
