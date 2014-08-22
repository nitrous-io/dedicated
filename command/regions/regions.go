package regions

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
Usage: nitro regions

  list available regions
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

type Region struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func init() {
	command.Register("regions", NewCommand)
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
	regions, err := c.fetchRegions()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if len(regions) == 0 {
		c.UI.Output("No regions found.")
		return 0
	}

	c.UI.Output(prettyPrint(regions))

	return 0
}

func (c *Command) Synopsis() string {
	return "List available regions"
}

func (c *Command) fetchRegions() ([]*Region, error) {
	resp, err := c.Client.Get(fmt.Sprintf("%s/api/v0/regions", c.Server), nil)
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Regions []*Region `json:"regions"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Regions, nil
}

func prettyPrint(regions []*Region) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	fmt.Fprintln(w, "Name", "\t", "Description")
	for _, r := range regions {
		fmt.Fprintln(w, r.Name, "\t", r.Description)
	}
	w.Flush()
	return b.String()
}
