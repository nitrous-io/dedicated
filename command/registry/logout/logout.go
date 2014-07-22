package logout

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
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
	UI cli.Ui
}

func init() {
	command.Register("registry:logout", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI: command.DefaultUI,
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

	if registry == "" {
		msg := "Docker registry is not specified, will log out from Docker Hub registry (yes|no) (default=yes):"
		confirm, err := c.UI.Ask(msg)
		if err != nil {
			c.UI.Error(fmt.Sprintf("Failed to get user's response: %v", err))
			return 1
		}
		confirm = strings.ToLower(confirm)
		if confirm == "no" {
			c.UI.Error("registry:logout is canceled. Please specify the desired Docker registry.")
			return 1
		}
	}
	registry = strings.TrimSpace(registry)

	err = registryLogout(registry, host)
	if err != nil {
		c.UI.Error(fmt.Sprintf("ERROR: %v", err))
		return 1
	}
	c.UI.Output(fmt.Sprintf("Successfully logged out from the Docker registry on %s\n", host))
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Login to a Docker registry on a Nitrous.IO host"
}

// log out from a Docker registry
// The host needs to run a freighterd daemon to accept auth request.
func registryLogout(registry, host string) error {
	params := url.Values{}
	params.Add("registry", registry)

	req, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s/registry/auth?%v", host, params.Encode()), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to log out from the Docker registry: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Failed to log out from the Docker registry: status %d", resp.StatusCode)
	}
	return nil
}
