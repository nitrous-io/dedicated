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
	"github.com/nitrous-io/nitro/consts"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

var usage = `
Usage: nitro registry <host>

	list currently logged in Docker registries.

`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("registry", NewCommand)
}

type Registry struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("registry", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		cmdFlags.Usage()
		return 1
	}
	host := args[0]

	c.UI.Output(fmt.Sprintf("Listing logged-in registries in '%s'...", host))
	registries, err := c.fetchRegistries(host)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to list registries (%v)", err))
		return 1
	}

	if len(registries) == 0 {
		c.UI.Output("You are not logged-in to any registries")
		return 0
	}

	t := PrettyPrint([]string{"Registry", "Username", "Email"}, registries...)
	c.UI.Output(t)

	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "List currently logged in Docker registries"
}

func (c *Command) fetchRegistries(host string) ([]*Registry, error) {
	resp, err := c.Client.GetAuthed(fmt.Sprintf("%s/api/v0/hosts/%s/registries", c.Server, host), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}

	j := struct {
		Registries []*Registry `json:"registries"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Registries, nil
}

func PrettyPrint(headers []string, registries ...*Registry) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	fmt.Fprintln(w, strings.Join(headers, "\t "))
	for _, r := range registries {
		fmt.Fprintln(w, r.URL, "\t", r.Username, "\t", r.Email)
	}
	w.Flush()
	return b.String()
}
