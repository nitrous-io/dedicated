package commit

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
	"github.com/nitrous-io/nitro/pkg/randomnamegenerator"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	tagUsage = "give image a name after committing (automatically generated if omitted)"
	Usage    = `
Usage: nitro commit <container slug>

  Commit a container to an image.

Options:

  -t, --tag=""    %s
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
	command.RegisterAuth("commit", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(fmt.Sprintf(Usage, tagUsage))
}

func (c *Command) Run(args []string) int {
	var imageName string
	cmdFlags := flag.NewFlagSet("commit", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&imageName, "tag", "t", "", tagUsage)

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
	if imageName == "" {
		imageName = randomnamegenerator.Generate("image")
	}

	c.UI.Output(fmt.Sprintf("Committing container '%s' to image '%s'", container, imageName))

	body, err := c.makeRequest(container, imageName)
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
	return "Commit container to an image"
}

func (c *Command) makeRequest(container, imageName string) (io.ReadCloser, error) {
	path := fmt.Sprintf("%s/api/v0/containers/%s/commit", c.Server, container)
	params := &url.Values{"name": []string{imageName}}
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
