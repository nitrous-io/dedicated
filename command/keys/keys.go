package keys

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/mitchellh/cli"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	ContainerUsage = "display only keys for this container name"
	Usage          = `
Usage: nitro keys

  list public keys on user containers

Options:

 -c, --container=""	%s
`
)

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

type Response struct {
	Keys []struct {
		ContainerID string `json:"container_id"`
		Fingerprint string `json:"fingerprint"`
		Key         string `json:"key"`
	}
}

func init() {
	command.Register("keys", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(fmt.Sprintf(Usage, ContainerUsage))
}

func (c *Command) Run(args []string) int {
	var container string

	cmdFlags := flag.NewFlagSet("keys", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&container, "container", "c", "", ContainerUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	params := &url.Values{}
	if container != "" {
		params.Add("container_id", container)
	}
	b, err := c.makeRequest(params)
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

func (c *Command) makeRequest(params *url.Values) (io.ReadCloser, error) {
	resp, err := c.Client.GetAuthed(fmt.Sprintf("%s/api/v0/%s", c.Server, "keys"), params)
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
	w.Init(b, 0, 0, 2, ' ', 0)
	defer func() {
		w.Flush()
		c.UI.Output(b.String())
	}()

	fmt.Fprintln(w, strings.Join([]string{
		"Container ID",
		"Fingerprint",
		"Key",
	}, "\t"))
	for _, k := range j.Keys {
		fmt.Fprintln(w, strings.Join([]string{
			k.ContainerID,
			k.Fingerprint,
			strings.TrimSpace(k.Key),
		}, "\t"))
	}
	return nil
}

func (c *Command) Synopsis() string {
	return "List public keys on user containers"
}
