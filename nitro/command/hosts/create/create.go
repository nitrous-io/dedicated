package create

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/command/hosts"
	"github.com/nitrous-io/dedicated/nitro/pkg/randomnamegenerator"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	sizeUsage   = "Size of the host (use `nitro plans` to see available sizes)"
	regionUsage = "A region to create the host (use `nitro regions` to see available regions)"
	slugUsage   = "A unique slug for the host"
)

const usage = `
Usage: nitro hosts:create <size> <region>

  create a new host

Options:

  size                  %s
  region                %s
  -s, --slug=""         %s
`

// response coming from nitrous server, we are only intested in
// - stream: messages emitted from hostbuilder
// - host: final host result coming from nitrous webapp
// - error_hash + error_description: error messages coming from nitrous webapp
type response struct {
	Stream string      `json:"stream"`
	Host   *hosts.Host `json:"host"`

	Error            string `json:"error"`
	ErrorHash        string `json:"error_hash"`
	ErrorDescription string `json:"error_description"`
}

func (r *response) error() string {
	msg := []string{}
	if r.Error != "" {
		msg = append(msg, fmt.Sprintf("error(%s)", r.Error))
	}
	if r.ErrorHash != "" {
		msg = append(msg, fmt.Sprintf("code(%s)", r.ErrorHash))
	}
	if r.ErrorDescription != "" {
		msg = append(msg, fmt.Sprintf("description(%s)", r.ErrorDescription))
	}
	return strings.Join(msg, " ")
}

type Command struct {
	UI     *command.BasicUI
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("hosts:create", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(fmt.Sprintf(usage, sizeUsage, regionUsage, slugUsage))
}

func (c *Command) Run(args []string) int {
	var (
		slug string
	)
	cmdFlags := flag.NewFlagSet("hosts:create", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&slug, "slug", "s", "", slugUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}
	args = cmdFlags.Args()
	if len(args) != 2 {
		c.UI.Output(c.Help())
		return 1
	}

	if slug == "" {
		slug = randomnamegenerator.Generate("host")
	}

	size := args[0]
	region := args[1]

	c.UI.Output(fmt.Sprintf("Creating new host '%s'. Please wait...", slug))

	host, err := c.createHost(size, region, slug, c.UI.Writer)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to create host - %v", err.Error()))
		return 1
	}

	c.UI.Output("Created new host:")
	t := hosts.PrettyPrint([]string{"Slug", "State", "Size"}, host)
	c.UI.Output(t)
	return 0
}

func (c *Command) Synopsis() string {
	return "Create a new host"
}

func (c *Command) createHost(size, region, slug string, out io.Writer) (*hosts.Host, error) {
	p := &url.Values{}
	p.Set("size", size)
	p.Set("region", region)
	p.Set("slug", slug)

	resp, err := c.Client.PostFormAuthed(fmt.Sprintf("%s/api/v0/%s", c.Server, "hosts"), p)
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	for {
		buildOutput := &response{}
		err = decoder.Decode(buildOutput)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("hosts/create: invalid format '%s'", err)
		}
		if buildOutput.Error != "" || buildOutput.ErrorDescription != "" || buildOutput.ErrorHash != "" {
			fmt.Fprintf(out, "===> %s | failed %s\n", slug, buildOutput.error())
		} else if buildOutput.Stream != "" {
			fmt.Fprintf(out, "===> %s | %s\n", slug, buildOutput.Stream)
		} else if buildOutput.Host != nil {
			return buildOutput.Host, nil
		}
	}
	return nil, errors.New("hosts/create: failed to return a host")
}
