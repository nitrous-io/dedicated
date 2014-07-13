package hostscreate

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
	regionUsage   = "Region in which you want to create the host (apse, use, usw, euw)"
	nameUsage     = "A unique name for the host"
	longnameUsage = "An optional name tag for the host"
)

const usage = `
Usage: nitro hosts:create <size>

  create a new host

Options:

	size - nano, micro, small, medium, large, xlarge, 2xlarge, 4xlarge
	-r, --region=""       %s 
	-n, --name=""         %s
	-L, --longname=""     %s

`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("hosts:create", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(fmt.Sprintf(usage, regionUsage, nameUsage, longnameUsage))
}

func (c *Command) Run(args []string) int {
	var (
		region   string
		name     string
		longname string
	)
	cmdFlags := flag.NewFlagSet("hosts:create", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&region, "region", "r", "", regionUsage)
	cmdFlags.StringVarP(&name, "name", "n", "", nameUsage)
	cmdFlags.StringVarP(&longname, "longname", "L", "", longnameUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}
	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Output(c.Help())
		return 1
	}

	size := args[0]

	c.UI.Output("Creating the host. Please wait...\n")
	err = c.createHost(size, region, name, longname)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to create the host - %v", err.Error()))
		return 1
	}

	c.UI.Output("Done.\n")
	return 0
}

func (c *Command) Synopsis() string {
	return "Creates a new host"
}

func (c *Command) createHost(size, region, name, longname string) error {
	p := &url.Values{}
	p.Set("size", size)
	p.Set("region", region)
	p.Set("name", name)
	p.Set("longname", longname)

	resp, err := c.Client.PostFormAuthed(fmt.Sprintf("%s/api/v0/%s", c.Server, "hosts"), p)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return httpclient.CheckResponse(resp, http.StatusCreated)
}
