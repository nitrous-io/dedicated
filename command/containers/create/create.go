package containerscreate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	flag "github.com/ogier/pflag"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/pkg/randomnamegenerator"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/nitrous-io/nitro/command/containers"
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
	params := &containerParams{}

	cmdFlags := flag.NewFlagSet("create", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&params.host, "host", "H", "", hostUsage)
	cmdFlags.StringVarP(&params.name, "name", "n", "", nameUsage)
	cmdFlags.StringVarP(&params.longname, "longname", "L", "", longnameUsage)
	cmdFlags.StringVarP(&params.image, "image", "i", "", imageUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	if params.name == "" {
		params.name = randomnamegenerator.Generate("container")
	}

	if params.host == "" {
		h := command.DefaultHost()
		if h == "" {
			c.UI.Error(fmt.Sprintf("Please specify a host."))
			c.UI.Output(c.Help())
			return 1
		}
		params.host = h
	}

	c.UI.Output(fmt.Sprintf("Creating container on '%s'. Please wait...", params.host))
	cnr, err := c.createContainer(params)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to create the container - %v", err.Error()))
		return 1
	}
	command.SaveDefaultHost(params.host)

	if cnr != nil {
		c.UI.Output(fmt.Sprintf("Created new container: '%s'", cnr.Name))
		t := PrettyPrint([]string{"Name", "State"}, cnr)
		c.UI.Output(t)
	} else {
		c.UI.Output("Done.")
	}
	return 0
}

func (c *Command) Synopsis() string {
	return "Create a new container"
}

func (c *Command) createContainer(cp *containerParams) (*Container, error) {
	p := &url.Values{}

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
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusCreated)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Container *Container `json:"container"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Container, nil
}
