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
	"github.com/nitrous-io/nitro/pkg/nitrousapi"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	noStdinUsage = "Do not attach STDIN"
)

var usage = fmt.Sprintf(`
Usage: nitro attach [SLUG]

Attach to a running container.

Options:

  --no-stdin=false     %s.
`, noStdinUsage)

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
	var noStdin bool

	cmdFlags := flag.NewFlagSet("attach", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.BoolVarP(&noStdin, "no-stdin", "", false, noStdinUsage)

	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Error(c.Help())
		return 1
	}
	slug := args[0]

	cr, err := nitrousapi.GetContainer(c.Client, c.Server, slug)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	var stdin io.Reader
	if !noStdin {
		stdin = c.UI.Reader
	}
	stdout := c.UI.Writer

	err = c.attach(cr, stdin, stdout)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Exited: %v", err))
		return 1
	}
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Attach to a running container."
}

func (c *Command) attach(cr *nitrousapi.ContainerAPI, stdin io.Reader, stdout io.Writer) error {
	host := fmt.Sprintf("%s:%d", cr.Host.IPAddress, cr.Host.FreighterdPort)

	dial, err := net.Dial("tcp", host)
	if err != nil {
		return fmt.Errorf("Can't connect to %s", host)
	}

	conn, err := httputil.NewClientConn(dial, nil), nil
	if err != nil {
		return err
	}
	defer conn.Close()

	req, err := http.NewRequest("POST", fmt.Sprintf("/containers/%s/attach", cr.CID), nil)
	if err != nil {
		return err
	}
	req.Host = host

	resp, _ := conn.Do(req)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed to attach to the remote shell for container %s, status %d",
			cr.CID, resp.StatusCode)
	}

	wc, r := conn.Hijack()
	defer wc.Close()

	done := make(chan error, 1)
	go func() {
		_, err = io.Copy(stdout, r)
		done <- err
	}()

	go func() {
		if stdin != nil {
			_, err = io.Copy(wc, stdin)
			done <- err
		}
	}()

	return <-done
}
