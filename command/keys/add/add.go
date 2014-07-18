package add

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	Usage = `
Usage: nitro keys:add <container_id> <key>

  container_id	the id of a container
  key			the path to a public key file on your local machine

  add a public key to a container
`
)

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.Register("keys:add", NewCommand)
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
	cmdFlags := flag.NewFlagSet("keys:add", flag.ContinueOnError)
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

	containerID := args[0]
	keyfile := args[1]

	if _, err := os.Stat(keyfile); err != nil {
		msg := err.Error()
		if os.IsNotExist(err) {
			msg = fmt.Sprintf("Could not find key at %s", keyfile)
		}
		c.UI.Error(msg)
		return 1
	}

	keybytes, err := ioutil.ReadFile(keyfile)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	err = c.makeRequest(&url.Values{
		"keytext":      {string(keybytes)},
		"container_id": {containerID},
	})
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	c.UI.Output(fmt.Sprintf("Added key at %s\n", keyfile))
	return 0
}

func (c *Command) makeRequest(data *url.Values) error {
	resp, err := c.Client.PutFormAuthed(fmt.Sprintf("%s/api/v0/%s", c.Server, "keys"),
		data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return command.CheckResponse(resp, http.StatusCreated)
}

func (c *Command) Synopsis() string {
	return "Add a public key to a container for SSH access"
}
