package tag

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/images"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const forceUsage = "overwrites an existing image with the new slug:tag even if it exists"

var usage = fmt.Sprintf(`
Usage: nitro images:tag <host> <image id or slug[:tag]> <new image slug[:tag]>

  Tag an existing image with a different slug and optional tag.

Options:

  -f, --force=""    %s
`, forceUsage)

type Response struct {
	Image images.Image `json:"image"`
}

type Command struct {
	UI     command.UI
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("images:tag", NewCommand)
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
	var force bool
	cmdFlags := flag.NewFlagSet("images:tag", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.BoolVarP(&force, "force", "f", false, forceUsage)

	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 3 {
		c.UI.Output(c.Help())
		return 1
	}

	host, image, newName := args[0], args[1], args[2]

	c.UI.Output(fmt.Sprintf("Tagging image '%s' as '%s' on host '%s'...", image, newName, host))

	if err := c.makeRequest(host, image, newName, force); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	// TODO would be useful to output image information -- need freighter to return
	c.UI.Output(fmt.Sprintf("Tagged image"))
	return 0
}

func (c *Command) Synopsis() string {
	return "Tag image on a host"
}

func (c *Command) makeRequest(host, image, newName string, force bool) error {
	path := fmt.Sprintf("%s/api/v0/hosts/%s/images/%s/tag", c.Server, host, image)
	params := &url.Values{}
	params.Set("tag", newName)
	if force {
		params.Set("force", "true")
	}
	resp, err := c.Client.PostFormAuthed(path, params)
	if err != nil {
		return err
	}
	return httpclient.CheckResponse(resp, http.StatusOK)
}
