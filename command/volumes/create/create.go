package create

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	flag "github.com/ogier/pflag"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/volumes"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const usage = `
Usage: nitro volumes:create <host_slug> <volume_slug>

  create a new data volume on host
`

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("volumes:create", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("create", flag.ContinueOnError)
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

	hostSlug, volSlug := args[0], args[1]
	c.UI.Output(fmt.Sprintf("Creating volume on '%s'. Please wait...", hostSlug))
	v, err := c.createVolume(hostSlug, volSlug)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to create the volume - %v", err.Error()))
		return 1
	}

	if v != nil {
		c.UI.Output(fmt.Sprintf("Created new volume: '%s'", v.Slug))
	}
	return 0
}

func (c *Command) Synopsis() string {
	return "Create a new volume"
}

func (c *Command) createVolume(hostSlug, volSlug string) (*volumes.Volume, error) {
	resp, err := c.Client.PostFormAuthed(
		fmt.Sprintf("%s/api/v0/hosts/%s/volumes", c.Server, hostSlug),
		&url.Values{"slug": []string{volSlug}})
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusCreated)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Volume *volumes.Volume `json:"volume"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Volume, nil
}
