package create

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	flag "github.com/ogier/pflag"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers"
	"github.com/nitrous-io/nitro/command/images"
	"github.com/nitrous-io/nitro/pkg/randomnamegenerator"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const (
	slugUsage         = "A unique slug for the container"
	imageUsage        = "Specify an image to use to create the container"
	envUsage          = "Set environment variables for the container in the format <key>:<value>. " + command.MultiargUsage
	volumesUsage      = "Add data volume so other containers can mount given <path>. " + command.MultiargUsage
	portForwardsUsage = "Forward ports from container to host.  Use one of: 'public:hostPort:containerPort', 'private:hostPort:containerPort', 'public::containerPort', or 'private::containerPort' specifications. Publicly forwarded ports will be accessible via the host. " + command.MultiargUsage
)

const usage = `
Usage: nitro create <host> [options]

  create a new container

Options:

  -s, --slug=""           %s
  -i, --image=""          %s
  -e, --env=[]            %s
  -v, --volume=[]         %s
  -p, --port-forward=[]   %s
`

type Command struct {
	UI     *cli.BasicUi
	Server string
	Client *httpclient.Client
}

type containerParams struct {
	host         string
	slug         string
	image        string
	envVars      command.Multiarg
	volumes      command.Multiarg
	portForwards command.Multiarg
}

func init() {
	command.RegisterAuth("create", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI: &cli.BasicUi{
			Reader: os.Stdin,
			Writer: os.Stdout,
		},
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Help() string {
	// TODO this is getting unwieldy, maybe text/template
	return strings.TrimSpace(fmt.Sprintf(usage, slugUsage,
		imageUsage, envUsage, volumesUsage, portForwardsUsage))
}

func (c *Command) Run(args []string) int {
	params := &containerParams{}

	cmdFlags := flag.NewFlagSet("create", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&params.slug, "slug", "s", "", slugUsage)
	cmdFlags.StringVarP(&params.image, "image", "i", "", imageUsage)

	cmdFlags.VarP(&params.envVars, "env", "e", envUsage)
	cmdFlags.VarP(&params.volumes, "volume", "v", volumesUsage)
	cmdFlags.VarP(&params.portForwards, "port-forward", "p", portForwardsUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Error(fmt.Sprintf("Please specify a host."))
		c.UI.Output(c.Help())
		return 1
	}
	params.host = args[0]

	if params.slug == "" {
		params.slug = randomnamegenerator.Generate("container")
	}

	stdout, ok := c.UI.Writer.(*os.File)
	if !ok {
		c.UI.Error("Requires a working terminal")
		return 1
	}

	// Step 1: Pull image.
	if err := c.pullImage(params.host, params.image, stdout, stdout.Fd()); err != nil {
		c.UI.Error(fmt.Sprintf("Failed to pull image: %s", err))
		return 1
	}

	// Step 2: Create container.
	c.UI.Output(fmt.Sprintf("Creating container on '%s'...", params.host))
	created, err := c.createContainer(params)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to create container: %s", err))
		return 1
	}

	// Step 3: Start container.
	started, err := c.start(params)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Container created but failed to start. Error: %s", err))
		c.UI.Output(containers.PrettyPrint([]string{"Slug", "Image", "State"}, created))
		return 1
	}

	c.UI.Output(fmt.Sprintf("Created new container: '%s'", started.Slug))
	c.UI.Output(containers.PrettyPrint([]string{"Slug", "Image", "State"}, started))

	return 0
}

func (c *Command) Synopsis() string {
	return "Create a new container"
}

func (c *Command) pullImage(host, image string, out io.Writer, termFd uintptr) error {
	path := fmt.Sprintf("%s/api/v0/hosts/%s/images/pull", c.Server, host)
	params := &url.Values{}
	params.Set("repository", image)
	params.Set("update", "false") // If image exists on host, don't update it.
	resp, err := c.Client.PostFormAuthed(path, params)
	if err != nil {
		return fmt.Errorf("failed to pull image: %s", err)
	}
	if err := httpclient.CheckResponse(resp, http.StatusOK); err != nil {
		return fmt.Errorf("unexpected response: %s", err)
	}
	defer resp.Body.Close()

	if err := images.DisplayJSONMessagesStream(resp.Body, out, termFd, true); err != nil {
		return fmt.Errorf("failed to display download progress: %s", err)
	}
	return nil
}

func (c *Command) createContainer(cp *containerParams) (*containers.Container, error) {
	p := &url.Values{}
	p.Set("host", cp.host)
	if cp.slug != "" {
		p.Set("slug", cp.slug)
	}
	if cp.image != "" {
		p.Set("image", cp.image)
	}
	for _, e := range cp.envVars {
		p.Add("env[]", e)
	}
	for _, v := range cp.volumes {
		p.Add("volume[]", v)
	}
	for _, pf := range cp.portForwards {
		p.Add("port[]", pf)
	}

	u := fmt.Sprintf("%s/api/v0/containers", c.Server)
	resp, err := c.Client.PostFormAuthed(u, p)
	if err != nil {
		return nil, err
	}
	if err := httpclient.CheckResponse(resp, http.StatusCreated); err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	j := struct {
		Container *containers.Container `json:"container"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}

	return j.Container, nil
}

func (c *Command) start(cp *containerParams) (*containers.Container, error) {
	p := &url.Values{}
	for _, e := range cp.envVars {
		p.Add("env[]", e)
	}
	for _, v := range cp.volumes {
		p.Add("volume[]", v)
	}
	for _, pf := range cp.portForwards {
		p.Add("port[]", pf)
	}

	u := fmt.Sprintf("%s/api/v0/containers/%s/start", c.Server, cp.slug)
	resp, err := c.Client.PostFormAuthed(u, p)
	if err != nil {
		return nil, err
	}
	return containers.ParseContainerResponse(resp)
}
