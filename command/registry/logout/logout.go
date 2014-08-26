package logout

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	registryUsage = "Docker registry's server. Defaults to Docker Hub registry"
)

var usage = fmt.Sprintf(`
Usage: nitro registry:logout [options] <host>

  log out from a Docker registry on a Nitrous.IO host.

Options:

  -r, --registry=""  %s
`, registryUsage)

type Command struct {
	UI     command.UI
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("registry:logout", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	var registry string
	var host string

	cmdFlags := flag.NewFlagSet("registry:logout", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&registry, "registry", "r", "", registryUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		cmdFlags.Usage()
		return 1
	}
	host = args[0]

	registry = strings.TrimSpace(registry)

	if registry == "" {
		prompt := "Docker registry is not specified, will log out from Docker Hub registry [y/N]:"
		errMsg := "Registry logout canceled."
		confirmation := c.UI.Confirm(prompt)
		switch confirmation {
		case command.ConfirmYes:
			// Use docker hub
			registry = "index.docker.io"
		case command.ConfirmNo:
			c.UI.Error(errMsg)
			return 1
		case command.ConfirmCancel:
			c.UI.Error(errMsg)
			return 130
		case command.ConfirmErr:
			c.UI.Error(fmt.Sprintf("Failed to get user's response: %v", err))
			return 1
		}
	}

	err = c.registryLogout(registry, host)
	if err != nil {
		c.UI.Error("Failed to logout from Docker registry")
		return 1
	}
	c.UI.Output(fmt.Sprintf("Successfully logged out from the Docker registry on %s\n", host))
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Log out from a Docker registry on a Nitrous.IO host"
}

func (c *Command) registryLogout(registry, host string) error {
	endpoint, err := url.Parse(fmt.Sprintf("%s/api/v0/hosts/%s/registries/", c.Server, host))
	if err != nil {
		return err
	}
	endpoint.Path += url.QueryEscape(registry)

	resp, err := c.Client.DeleteAuthed(endpoint.String(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return httpclient.CheckResponse(resp, http.StatusOK)
}
