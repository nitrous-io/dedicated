package list

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"

	. "github.com/nitrous-io/nitro/command/images"
)

const Usage = `
Usage: nitro images <host>

  list images on a given host
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("images", NewCommand)
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
	cmdFlags := flag.NewFlagSet("images", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	var host string
	args = cmdFlags.Args()
	switch len(args) {
	case 1:
		host = args[0]
	default:
		c.UI.Output(c.Help())
		return 1
	}

	c.UI.Output(fmt.Sprintf("Listing images on '%s'...", host))
	images, err := c.fetchImages(host)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if len(images) == 0 {
		c.UI.Output("No images found.")
		return 0
	}

	t := PrettyPrint(images...)
	c.UI.Output(t)

	return 0
}

func (c *Command) Synopsis() string {
	return "List images on a host"
}

func (c *Command) fetchImages(host string) ([]*Image, error) {
	reqURL := fmt.Sprintf("%s/api/v0/hosts/%s/images", c.Server, host)
	resp, err := c.Client.GetAuthed(reqURL, nil)
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Images []*Image `json:"images"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Images, nil
}
