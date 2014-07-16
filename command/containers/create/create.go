package create

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	flag "github.com/ogier/pflag"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const (
	hostUsage     = "Specify a host to create the container"
	nameUsage     = "A unique name for the container"
	longnameUsage = "An optional name tag for the container"
	imageUsage    = "Specify an image to use to create the container"
)

const usage = `
Usage: nitro create [options]

  create a new container

Options:

  -H, --host=""        %s
  -n, --name=""        %s
  -L, --longname=""    %s
  -i, --image=""       %s
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

type containerParams struct {
	host     string
	name     string
	longname string
	image    string
}

func init() {
	command.Register("create", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(fmt.Sprintf(usage, hostUsage, nameUsage, longnameUsage, imageUsage))
}

func (c *Command) Run(args []string) int {
	cp := &containerParams{}

	cmdFlags := flag.NewFlagSet("create", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&cp.host, "host", "H", "", hostUsage)
	cmdFlags.StringVarP(&cp.name, "name", "n", "", nameUsage)
	cmdFlags.StringVarP(&cp.longname, "longname", "L", "", longnameUsage)
	cmdFlags.StringVarP(&cp.image, "image", "i", "", imageUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	c.UI.Output("Creating the container. Please wait...\n")
	err = c.createContainer(cp)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to create the container - %v", err.Error()))
		return 1
	}

	c.UI.Output("Done.\n")
	return 0
}

func (c *Command) Synopsis() string {
	return "Create a new container"
}

func (c *Command) createContainer(cp *containerParams) error {
	p := &url.Values{}

	if cp.host == "" {
		// TODO: when host is not specified pick the last accessed host
		return errors.New("host required")
	}

	if cp.name != "" {
		p.Set("name", cp.name)
	}

	if cp.longname != "" {
		p.Set("longname", cp.longname)
	}

	if cp.image != "" {
		p.Set("image", cp.image)
	}

	resp, err := c.Client.PostFormAuthed(fmt.Sprintf("%s/api/v0/hosts/%s/containers", c.Server, cp.host), p)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return httpclient.CheckResponse(resp, http.StatusCreated)
}
