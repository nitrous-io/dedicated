package plans

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/consts"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const usage = `
Usage: nitro plans

  list available plans
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

type Plan struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func init() {
	command.Register("plans", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(""),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Run(args []string) int {
	plans, err := c.fetchPlans()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if len(plans) == 0 {
		c.UI.Output("No plans found.")
		return 0
	}

	c.UI.Output(prettyPrint(plans))

	return 0
}

func (c *Command) Synopsis() string {
	return "List available plans"
}

func (c *Command) fetchPlans() ([]*Plan, error) {
	resp, err := c.Client.Get(fmt.Sprintf("%s/api/v0/plans", c.Server), nil)
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Plans []*Plan `json:"plans"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Plans, nil
}

func prettyPrint(plans []*Plan) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	fmt.Fprintln(w, "Slug", "\t", "Name")
	for _, p := range plans {
		fmt.Fprintln(w, p.Slug, "\t", p.Name)
	}
	w.Flush()
	return b.String()
}
