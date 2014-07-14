package list

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const usage = `
Usage: nitro list <host>

  list containers in the host
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

type Container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
}

func init() {
	command.Register("list", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
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

	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Output(c.Help())
		return 1
	}

	host := args[0]

	containers, err := c.fetchContainers(host)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if len(containers) == 0 {
		c.UI.Output("No containers found\n")
		return 0
	}

	t := formattedOutput([]string{"Name", "State"}, containers)
	c.UI.Output(t)

	return 0
}

func (c *Command) Synopsis() string {
	return "List containers"
}

func (c *Command) fetchContainers(host string) ([]Container, error) {
	resp, err := c.Client.GetAuthed(fmt.Sprintf("%s/api/v0/hosts/%s/containers", c.Server, host), nil)
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Containers []Container `json:"containers"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Containers, nil
}

func formattedOutput(headers []string, containers []Container) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	w.Init(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t "))
	for _, r := range containers {
		fmt.Fprintln(w,
			r.Name, "\t",
			r.State,
		)
	}
	w.Flush()
	return b.String()
}
