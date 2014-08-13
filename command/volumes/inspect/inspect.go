package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/mitchellh/cli"
	flag "github.com/ogier/pflag"

	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/volumes"
	"github.com/nitrous-io/nitro/consts"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const usage = `
Usage: nitro volumes:inspect <host_slug> <volume_slug>

  inspect data volume on host

Options:

  -h, --human-readable     %s
`

const humanUsage = "print sizes in human readable format (e.g., 1K 234M 5G)"

var humanFlag bool

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

type Response struct {
	Volume volumes.Volume `json:"volume"`
}

func init() {
	command.RegisterAuth("volumes:inspect", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(fmt.Sprintf(usage, humanUsage))
}

func (c *Command) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("list", flag.ContinueOnError)
	cmdFlags.BoolVarP(&humanFlag, "human-readable", "h", false, humanUsage)
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

	c.UI.Output(fmt.Sprintf("Inspecting volume %s on '%s'...", volSlug, hostSlug))
	b, err := c.makeRequest(hostSlug, volSlug)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	err = c.handleResponse(b)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *Command) Synopsis() string {
	return "List volumes"
}

func (c *Command) makeRequest(hostSlug, volSlug string) (io.ReadCloser, error) {
	resp, err := c.Client.GetAuthed(fmt.Sprintf("%s/api/v0/hosts/%s/volumes/%s", c.Server, hostSlug, volSlug), nil)
	if err != nil {
		return nil, err
	}
	err = httpclient.CheckResponse(resp, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Command) handleResponse(body io.ReadCloser) error {
	defer body.Close()
	j := Response{}
	err := json.NewDecoder(body).Decode(&j)
	if err != nil {
		return err
	}

	// tabwriter is overkill at present because only one column present, but
	// easy to add in future
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}
	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	defer func() {
		w.Flush()
		c.UI.Output(b.String())
	}()

	sizeHeader, size := "Size (bytes)", fmt.Sprintf("%d", j.Volume.Size)
	if humanFlag {
		sizeHeader = "Size"
		size = humanReadable(j.Volume.Size)
	}
	fmt.Fprintln(w, strings.Join([]string{"Slug", sizeHeader}, "\t"))
	fmt.Fprintln(w, strings.Join([]string{j.Volume.Slug, size}, "\t"))
	return nil
}

func humanReadable(size uint64) string {
	if size == 0 {
		return "0"
	}
	// yes, it's smaller than uint64 but disks with that amount of space won't
	// exist for some time
	fsize := float64(size)

	units := []string{"", "K", "M", "G", "T"}
	fexp := math.Log(fsize) / math.Log(1024)
	exp := int(math.Trunc(fexp))
	if exp > len(units)-1 {
		exp = len(units) - 1
	}
	fsize /= math.Pow(1024, float64(exp))
	out := fmt.Sprintf("%.1f%s", fsize, units[exp])
	if strings.HasSuffix(out, ".0") {
		out = out[:len(out)-2]
	}
	return out
}
