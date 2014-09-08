package shell

import (
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers"
	"github.com/nitrous-io/nitro/pkg/nitrousapi"
	"github.com/nitrous-io/nitro/pkg/protoshell"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	usernameUsage = "Run as this user"
	defaultTerm   = "xterm-256color"
)

var usage = fmt.Sprintf(`
Usage: nitro shell <slug>

  open a shell on a container

Options:

  -u, --username=""    %s.
`, usernameUsage)

func init() {
	command.RegisterAuth("shell", NewCommand)
}

type Command struct {
	UI     *cli.BasicUi
	client *httpclient.Client
	server string
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI: &cli.BasicUi{
			Reader: os.Stdin,
			Writer: os.Stdout,
		},
		server: command.Env.APIURL,
		client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	var username string
	cmdFlags := flag.NewFlagSet("shell", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&username, "username", "u", "", usernameUsage)

	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Error(c.Help())
		return 1
	}

	slug := args[0]

	cr, err := nitrousapi.GetContainer(c.client, c.server, slug)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	if cr.State != "running" {
		c.UI.Error("Requires a running container")
		return 1
	}

	// prepare the term
	stdin := c.UI.Reader
	stdout := c.UI.Writer
	pty, ok := stdout.(*os.File)
	if !ok {
		c.UI.Error("Requires a working terminal")
		return 1
	}

	host := fmt.Sprintf("%s:%d", cr.Host.IPAddress, cr.Host.FreighterdPort)
	tlsConfig, err := command.ClientConfig(cr.Host.Hostname)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error establishing secure connection to %s", host))
		return 1
	}

	command := getLoginCommand(username)
	sh := protoshell.New(host, cr.CID, command, func() (string, error) {
		return containers.GetToken(slug, c.server, []string{"shell"}, c.client)
	})
	if err := sh.Run(pty.Fd(), stdin, stdout); err != nil {
		c.UI.Error(fmt.Sprintf("Exited: %v", err))
		return 1
	}
	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Open a shell on a container"
}

func getLoginCommand(username string) string {
	if username == "" {
		username = "root"
	}
	term := os.Getenv("TERM")
	if term == "" {
		term = defaultTerm
	}
	// use getty autologin to get a tty (so screen/tmux can run) because
	// lxc-attach will not allocate a new one
	// we use direct path here incase /sbin is not set in $PATH for some image
	//
	// if getty fails, we fall back to /bin/sh. This typically happens on
	// system that doesnt bundle util-linux such as busybox. In those case, we
	// just dont have a sane way to getty and autologin so we will just perform a
	// normal login without getty
	return fmt.Sprintf("/sbin/getty --noclear -i -a %s -8 38400 tty %s 2>/dev/null -o \"-f %s -p\" || /bin/login -f %s -p TERM=%s", username, term, username, username, term)
}
