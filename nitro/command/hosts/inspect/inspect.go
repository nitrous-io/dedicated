package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/consts"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const jsonUsage = "Display output in JSON format"

var usage = fmt.Sprintf(`
Usage: nitro inspect <host>

  inspect a host

Options:

  -j, --json    %s
`, jsonUsage)

type Command struct {
	UI     cli.Ui
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("hosts:inspect", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Run(args []string) int {
	var showJSON bool

	cmdFlags := flag.NewFlagSet("hosts:inspect", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.BoolVarP(&showJSON, "json", "j", false, jsonUsage)

	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Output(c.Help())
		return 1
	}

	slug := args[0]
	h, err := c.inspectHost(slug)
	if err != nil {
		c.printError(showJSON, err.Error())
		return 1
	}
	s := c.getHostStats(slug)

	if showJSON {
		j := struct {
			Host  *host  `json:"host,omitempty"`
			Stats *stats `json:"stats,omitempty"`
		}{
			Host:  h,
			Stats: s,
		}
		b, err := json.Marshal(j)
		if err != nil {
			c.printError(showJSON, "An unexpected error has occurred.")
			return 1
		}
		c.UI.Output(string(b))
	} else {
		c.UI.Output(c.prettyPrint(h, s))
	}

	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Inspect a host"
}

type (
	host struct {
		Slug      string `json:"slug"`
		IPAddress string `json:"ip_address"`
		State     string `json:"state"`
		Size      string `json:"size"`
		Hostname  string `json:"hostname"`
	}

	stats struct {
		Uptime       uint64    `json:"uptime"`
		MemoryUsage  uint64    `json:"memory_usage"`
		MemoryTotal  uint64    `json:"memory_total"`
		StorageUsage uint64    `json:"storage_usage"`
		StorageTotal uint64    `json:"storage_total"`
		NumCPU       int       `json:"num_cpu"`
		PerCPUUsage  []float64 `json:"per_cpu_usage"`
	}
)

func (c *Command) inspectHost(slug string) (*host, error) {
	url := fmt.Sprintf("%s/api/v0/hosts/%s", c.Server, slug)
	resp, err := c.Client.GetAuthed(url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := httpclient.CheckResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var j struct {
		Host *host `json:"host"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}
	return j.Host, nil
}

func (c *Command) getHostStats(slug string) *stats {
	url := fmt.Sprintf("%s/api/v0/hosts/%s/stats", c.Server, slug)
	resp, err := c.Client.GetAuthed(url, nil)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if err := httpclient.CheckResponse(resp, http.StatusOK); err != nil {
		return nil
	}

	var j struct {
		Stats *stats `json:"stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil
	}
	return j.Stats
}

func (c *Command) printError(showJSON bool, msg string) {
	if showJSON {
		b, err := json.Marshal(msg)
		if err != nil {
			c.UI.Error(`{"error": "An unexpected error has occurred."}`)
			return
		}
		c.UI.Error(fmt.Sprintf(`{"error": %s}`, string(b)))
	} else {
		c.UI.Error(msg)
	}
}

func (c *Command) prettyPrint(h *host, s *stats) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	headers := []string{"Slug", "State", "Size", "IP", "Hostname"}
	values := []interface{}{h.Slug, h.State, h.Size, h.IPAddress, h.Hostname}

	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	fmt.Fprint(w, strings.Join(headers, "\t"), "\n")
	fmt.Fprint(w, insertAfterEach(values, "\t")...)
	fmt.Fprintln(w)

	if s != nil {
		headers = []string{
			"Uptime", "Memory Used (bytes)", "Memory Total (bytes)",
			"Storage Used (bytes)", "Storage Total (bytes)", "Number of CPUs",
		}
		values = []interface{}{
			s.Uptime, s.MemoryUsage, s.MemoryTotal,
			s.StorageUsage, s.StorageTotal, s.NumCPU,
		}
		for i, usage := range s.PerCPUUsage {
			headers = append(headers, fmt.Sprintf("CPU %d Usage", i))
			values = append(values, fmt.Sprintf("%.2f %%", usage))
		}
		fmt.Fprint(w, "\n", strings.Join(headers, "\t"), "\n")
		fmt.Fprint(w, insertAfterEach(values, "\t")...)
		fmt.Fprintln(w)
	}

	w.Flush()
	return b.String()
}

func insertAfterEach(a []interface{}, sep interface{}) []interface{} {
	n := len(a)
	if n == 0 || n == 1 {
		return a
	}
	b := []interface{}{}
	for _, elem := range a {
		b = append(b, elem, sep)
	}
	return b
}
