package run

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers"
	"github.com/nitrous-io/nitro/pkg/nitrousapi"
	"github.com/nitrous-io/nitro/pkg/protoshell"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const usage = `
Usage: nitro run [container] [cmd]

  run a command on a container
`

func init() {
	command.RegisterAuth("run", NewCommand)
}

type Command struct {
	UI     *cli.BasicUi
	client *httpclient.Client
	server string
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI: &cli.BasicUi{
			Reader: os.Stdin,
			Writer: os.Stdout,
		},
		server: command.Env.APIURL,
		client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("run", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) < 2 {
		c.UI.Error(c.Help())
		return 1
	}

	slug := args[0]

	cr, err := nitrousapi.GetContainer(c.client, c.server, slug)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	if cr.State != "running" {
		c.UI.Error("Requires a running container")
		return 1
	}

	host := fmt.Sprintf("%s:%d", cr.Host.IPAddress, cr.Host.FreighterdPort)
	tlsConfig, err := command.ClientConfig(cr.Host.Hostname)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error establishing secure connection to %s", cr.Host.Hostname))
		return 1
	}
	command := strings.Join(args[1:], " ")

	// prepare the term
	stdin, ok := c.UI.Reader.(*os.File)
	if !ok {
		c.UI.Error("Requires a working terminal")
		return 1
	}
	stdout, ok := c.UI.Writer.(*os.File)
	if !ok {
		c.UI.Error("Requires a working terminal")
		return 1
	}

	sh := protoshell.New(host, cr.CID, command, tlsConfig, func() (string, error) {
		return containers.GetToken(slug, c.server, []string{"shell"}, c.client)
	})
	if err := sh.Run(stdout.Fd(), stdin, stdout); err != nil {
		c.UI.Error(fmt.Sprintf("Exited: %v", err))
		return 1
	}
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Run a command on a container"
}
