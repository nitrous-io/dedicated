package list

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/mitchellh/cli"

	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/consts"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	Usage = `
Usage: nitro keys <container>

  list public keys in the container
`
)

type Command struct {
	UI     command.UI
	Server string
	Client *httpclient.Client
}

type Response struct {
	Keys []struct {
		Fingerprint string `json:"fingerprint"`
		Key         string `json:"key"`
	}
}

func init() {
	command.RegisterAuth("keys", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Help() string {
	return Usage
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("keys", flag.ContinueOnError)
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

	container := args[0]

	b, err := c.makeRequest(container)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	err = c.handleResponse(b)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	return 0
}

func (c *Command) makeRequest(container string) (io.ReadCloser, error) {
	resp, err := c.Client.GetAuthed(fmt.Sprintf("%s/api/v0/containers/%s/keys", c.Server, container), nil)
	if err != nil {
		return nil, err
	}
	err = command.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Command) handleResponse(body io.ReadCloser) error {
	defer body.Close()
	j := Response{}
	err := json.NewDecoder(body).Decode(&j)
	if err != nil {
		return err
	}
	if len(j.Keys) == 0 {
		c.UI.Output("No keys found")
		return nil
	}

	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}
	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	defer func() {
		w.Flush()
		c.UI.Output(b.String())
	}()

	fmt.Fprintln(w, strings.Join([]string{
		"Fingerprint",
		"Key",
	}, "\t"))
	for _, k := range j.Keys {
		fmt.Fprintln(w, strings.Join([]string{
			k.Fingerprint,
			strings.TrimSpace(k.Key),
		}, "\t"))
	}
	return nil
}

func (c *Command) Synopsis() string {
	return "List public keys in container"
}
