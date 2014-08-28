package login

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/bgentry/speakeasy"
	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	usernameUsage      = "Docker registry's username"
	passwordUsage      = "Docker registry's password"
	emailUsage         = "Docker registry's email"
	registryUsage      = "Docker registry's server. Defaults to Docker Hub registry"
	defaultRegistryURL = "https://index.docker.io/v1/"
)

var usage = fmt.Sprintf(`
Usage: nitro registry:login [options] <host>

  log in to a Docker registry on a Nitrous.IO host.

Options:
  -u, --username=""   %s
  -p, --password=""   %s
  -e, --email=""      %s
  -r, --registry=""   %s
`, usernameUsage, passwordUsage, emailUsage, registryUsage)

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("registry:login", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	var username string
	var password string
	var email string
	var registryURL string
	var host string

	cmdFlags := flag.NewFlagSet("registry:login", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&username, "username", "u", "", usernameUsage)
	cmdFlags.StringVarP(&password, "password", "p", "", passwordUsage)
	cmdFlags.StringVarP(&email, "email", "e", "", emailUsage)
	cmdFlags.StringVarP(&registryURL, "registry", "r", "", registryUsage)

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
			if err.Error() == "interrupted" {
				c.UI.Error("Login cancelled.")
				return 130
			}
			c.UI.Error(fmt.Sprintf("An unexpected error has occurred: %v", err))
		}
	}
	if password == "" {
		password, err = speakeasy.Ask("Password (will not be echoed): ")
		if err != nil {
			if err.Error() == "interrupted" {
				c.UI.Error("Login cancelled.")
				return 130
			}
			c.UI.Error(fmt.Sprintf("An unexpected error has occurred: %v", err))
		}
	}
	if email == "" {
		email, err = c.UI.Ask("Email:")
		if err != nil {
			if err.Error() == "interrupted" {
				c.UI.Error("Login cancelled.")
				return 130
			}
			c.UI.Error(fmt.Sprintf("An unexpected error has occurred: %v", err))
		}
	}

	if registryURL == "" {
		registryURL, err = c.UI.Ask("Registry (press enter for index.docker.io): ")
		if err != nil {
			if err.Error() == "interrupted" {
				c.UI.Error("Registry login canceled.")
				return 130
			}
			c.UI.Error(fmt.Sprintf("An unexpected error has occurred: %v", err))
		}
		// user has pressed enter, use index.docker.io
		if registryURL == "" {
			registryURL = "index.docker.io"
		}
	}

	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	email = strings.TrimSpace(email)
	registryURL = strings.TrimSpace(registryURL)

	err = c.registryLogin(username, password, email, registryURL, host)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to login to Docker registry (%v)", err))
		return 1
	}
	c.UI.Output(fmt.Sprintf("Successfully logged in to Docker registry on %s\n", host))
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Log in to a Docker registry on a Nitrous.IO host"
}

func (c *Command) registryLogin(username, password, email, registryURL, host string) error {
	p := &url.Values{}
	p.Add("username", username)
	p.Add("password", password)
	p.Add("email", email)

	if registryURL != "" {
		p.Add("url", registryURL)
	}

	resp, err := c.Client.PostFormAuthed(fmt.Sprintf("%s/api/v0/hosts/%s/registries", c.Server, host), p)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return httpclient.CheckResponse(resp, http.StatusOK)
}
