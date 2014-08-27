package attach

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers"
	"github.com/nitrous-io/nitro/pkg/nitrousapi"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

var usage = `
Usage: nitro attach <container slug>

  attach to a running container
`

type Command struct {
	UI     *cli.BasicUi
	Client *httpclient.Client
	Server string
}

func init() {
	command.RegisterAuth("attach", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI: &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stderr,
		},
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	if len(args) != 1 {
		c.UI.Error(c.Help())
		return 1
	}
	slug := args[0]

	token, err := containers.GetToken(slug, c.Server, []string{"attach"}, c.Client)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	cn, err := nitrousapi.GetContainer(c.Client, c.Server, slug)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	host := fmt.Sprintf("%s:%d", cn.Host.IPAddress, cn.Host.FreighterdPort)
	if err := c.attach(host, token, cn.CID, c.UI.Writer); err != nil {
		c.UI.Error(fmt.Sprintf("Exited: %v", err))
		return 1
	}
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Attach to a running container"
}

func (c *Command) attach(host, token, containerID string, stdout io.Writer) error {
	dial, err := net.Dial("tcp", host)
	if err != nil {
		return fmt.Errorf("Can't connect to host %s", host)
	}

	conn := httputil.NewClientConn(dial, nil)
	defer conn.Close()

	req, err := http.NewRequest("POST", fmt.Sprintf("/containers/%s/attach", containerID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = host

	resp, _ := conn.Do(req)
	switch resp.StatusCode {
	case http.StatusOK:
		// Pass.
	case http.StatusUnauthorized:
		return fmt.Errorf("You do not have permission to attach to the container.")
	default:
		return fmt.Errorf("Failed to attach to the container. Make sure that it's running.")
	}

	wc, r := conn.Hijack()
	defer wc.Close()

	_, err = io.Copy(stdout, r)
	return err
}
