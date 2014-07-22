package login

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/bgentry/speakeasy"
	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	flag "github.com/ogier/pflag"
)

const (
	usernameUsage = "Docker registry's username"
	passwordUsage = "Docker registry's password"
	emailUsage    = "Docker registry's email"
	registryUsage = "Docker registry's server"
)

var usage = fmt.Sprintf(`
Usage: nitro registry:login [options] <host>

  login to a Docker registry on a Nitrous.IO host.

Options:
  -u, --username=""   %s
  -p, --password=""   %s
  -e, --email=""      %s
  -r, --registry=""   %s
`, usernameUsage, passwordUsage, emailUsage, registryUsage)

type Command struct {
	UI cli.Ui
}

func init() {
	command.Register("registry:login", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI: command.DefaultUI,
	}, nil
}

func (c *Command) Run(args []string) int {
	var username string
	var password string
	var email string
	var registry string
	var host string

	cmdFlags := flag.NewFlagSet("registry:login", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&username, "useranme", "u", "", usernameUsage)
	cmdFlags.StringVarP(&password, "password", "p", "", passwordUsage)
	cmdFlags.StringVarP(&email, "email", "e", "", emailUsage)
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

	if username == "" {
		username, err = c.UI.Ask("Username:")
		if err != nil {
			c.UI.Error(fmt.Sprintf("Failed to get user's username: %v", err))
		}
	}
	if password == "" {
		password, err = speakeasy.Ask("Password (will not be echoed): ")
		if err != nil {
			c.UI.Error(fmt.Sprintf("An unexpected error has occurred: %v", err))
		}
	}
	if email == "" {
		email, err = c.UI.Ask("Email:")
		if err != nil {
			c.UI.Error(fmt.Sprintf("Failed to get user's email: %v", err))
		}
	}
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	email = strings.TrimSpace(email)
	registry = strings.TrimSpace(registry)

	err = registryLogin(username, password, email, registry, host)
	if err != nil {
		c.UI.Error(fmt.Sprintf("ERROR: %v", err))
		return 1
	}
	c.UI.Output(fmt.Sprintf("Successfully logged in to Docker registry on %s\n", host))
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Login to a Docker registry on a Nitrous.IO host"
}

// login to a Docker registry on the specified host
// The host needs to run a freighterd daemon to accept auth request.
func registryLogin(username, password, email, registry, host string) error {
	params := url.Values{}
	params.Add("username", username)
	params.Add("password", password)
	params.Add("email", email)
	params.Add("registry", registry)

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/registry/auth?%v", host, params.Encode()), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to login to the Docker registry: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Failed to login to the Docker registry: status %d", resp.StatusCode)
	}
	return nil
}
