package list

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/command/hosts"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const usage = `
Usage: nitro hosts

  list all hosts
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("hosts", NewCommand)
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
	cmdFlags := flag.NewFlagSet("hosts", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	hsts, err := c.fetchHosts()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if len(hsts) == 0 {
		c.UI.Output("No hosts found.")
		return 0
	}

	t := hosts.PrettyPrint([]string{"Slug", "State", "Size", "IP Address"}, hsts...)
	c.UI.Output(t)

	return 0
}

func (c *Command) Synopsis() string {
	return "List all hosts"
}

func (c *Command) fetchHosts() ([]*hosts.Host, error) {
	resp, err := c.Client.GetAuthed(fmt.Sprintf("%s/api/v0/hosts", c.Server), nil)
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Hosts []*hosts.Host `json:"hosts"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Hosts, nil
}
