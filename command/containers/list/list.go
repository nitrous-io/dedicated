package list

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	flag "github.com/ogier/pflag"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/nitrous-io/nitro/command/containers"
)

const usage = `
Usage: nitro list [host]

  list containers
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("list", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("list", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	var host string
	args = cmdFlags.Args()
	if len(args) == 1 {
		host = args[0]
	}

	if host != "" {
		c.UI.Output(fmt.Sprintf("Listing containers on '%s'...", host))
	} else {
		c.UI.Output("Listing all containers...")
	}
	containers, err := c.fetchContainers(host)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if len(containers) == 0 {
		c.UI.Output("No containers found.")
		return 0
	}

	t := PrettyPrint([]string{"Name", "Image", "State"}, containers...)
	c.UI.Output(t)

	return 0
}

func (c *Command) Synopsis() string {
	return "List containers"
}

func (c *Command) fetchContainers(host string) ([]*Container, error) {
	params := &url.Values{}
	endpoint := fmt.Sprintf("%s/api/v0/containers", c.Server)
	if host != "" {
		params.Set("host", host)
	}
	resp, err := c.Client.GetAuthed(endpoint, params)
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Containers []*Container `json:"containers"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Containers, nil
}
