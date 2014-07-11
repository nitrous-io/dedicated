package keysremove

import (
	"fmt"
	"github.com/mitchellh/cli"
	"net/http"
	"net/url"
	"strings"

	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"

	flag "github.com/ogier/pflag"
)

const (
	ContainerUsage = "only remove key from this container"
	Usage          = `
Usage: nitro keys:remove <md5_fingerprint>

  md5_fingerprint	the fingerprint of the key to remove

  remove a public key

Options:

-c, --container=""	%s
`
)

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("keys:remove", NewCommand)
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

	cmdFlags := flag.NewFlagSet("keys:add", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&container, "container", "c", "", ContainerUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}
	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Output(c.Help())
		return 1
	}

	fingerprint := args[0]
	params := &url.Values{}
	if container != "" {
		params.Add("container_id", container)
	}

	err = c.makeRequest(fingerprint, params)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	var status string
	if container == "" {
		status = fmt.Sprintf("Removed all instances of key %s", fingerprint)
	} else {
		status = fmt.Sprintf("Removed key %s from container %s", fingerprint, container)
	}
	c.UI.Output(status + "\n")
	return 0
}

func (c *Command) makeRequest(fingerprint string, params *url.Values) error {
	path := fmt.Sprintf("%s/api/v0/%s/%s", c.Server, "keys", fingerprint)
	resp, err := c.Client.DeleteAuthed(path, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return httpclient.CheckResponse(resp, http.StatusOK)
}

func (c *Command) Synopsis() string {
	return "Remove a public key"
}
