package pull

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/images"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	Usage = `
Usage: nitro images:pull <host> [registry_host/][username/]<repo_name:tag>

  pull image onto a given host
`
)

type Response struct {
	Image images.Image `json:"image"`
}

type Command struct {
	UI     command.UI
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("images:pull", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(Usage)
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("images:pull", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 2 {
		c.UI.Output(c.Help())
		return 1
	}

	host, image := args[0], args[1]

	c.UI.Output(fmt.Sprintf("Pulling image '%s' to '%s'...", image, host))

	body, err := c.makeRequest(host, image)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	err = c.handleResponse(body)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *Command) Synopsis() string {
	return "Pull image onto a host"
}

func (c *Command) makeRequest(host, image string) (io.ReadCloser, error) {
	path := fmt.Sprintf("%s/api/v0/hosts/%s/images/pull", c.Server, host)
	params := &url.Values{"repository": []string{image}}
	resp, err := c.Client.PostFormAuthed(path, params)
	if err != nil {
		return nil, err
	}
	return resp.Body, httpclient.CheckResponse(resp, http.StatusOK)
}

func (c *Command) handleResponse(body io.ReadCloser) error {
	defer body.Close()
	j := Response{}
	err := json.NewDecoder(body).Decode(&j)
	if err != nil {
		return err
	}
	c.UI.Output(images.PrettyPrint(&j.Image))
	return nil
}
