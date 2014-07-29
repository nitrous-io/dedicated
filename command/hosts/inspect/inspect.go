package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/consts"
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
	command.Register("hosts:inspect", NewCommand)
}

func NewCommand() (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(command.Env.AuthToken),
	}, nil
}

func (c *Command) Run(args []string) int {
	var showJSON bool

	cmdFlags := flag.NewFlagSet("hosts:inspect", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.BoolVarP(&showJSON, "json", "j", false, jsonUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 1 {
		c.UI.Output(c.Help())
		return 1
	}

	slug := args[0]
	host, err := c.inspectHost(slug)
	if err != nil {
		c.printError(showJSON, err.Error())
		return 1
	}
	if showJSON {
		b, err := json.Marshal(host)
		if err != nil {
			c.printError(showJSON, "An unexpected error has occurred.")
			return 1
		}
		c.UI.Output(string(b))
	} else {
		c.UI.Output(c.prettyPrint(host))
	}

	return 0
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Synopsis() string {
	return "Inspect a host"
}

type response struct {
	Host *struct {
		Slug      string `json:"slug"`
		IPAddress string `json:"ip_address"`
		State     string `json:"state"`
		Hostname  string `json:"hostname"`
		Region    *struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"region"`
		Plan *struct {
			Name string `json:"name"`
		} `json:"plan"`
		Stats *struct {
			Uptime       uint64    `json:"uptime"`
			MemoryUsage  uint64    `json:"memory_usage"`
			MemoryTotal  uint64    `json:"memory_total"`
			StorageUsage uint64    `json:"storage_usage"`
			StorageTotal uint64    `json:"storage_total"`
			NumCPU       int       `json:"num_cpu"`
			PerCPUUsage  []float64 `json:"per_cpu_usage"`
		} `json:"stats,omitempty"`
	} `json:"host"`
}

func (c *Command) inspectHost(slug string) (*response, error) {
	url := fmt.Sprintf("%s/api/v0/hosts/%s", c.Server, slug)
	resp, err := c.Client.GetAuthed(url, nil)
	if err != nil {
		return nil, err
	}

	var j response
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return &j, nil
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

func (c *Command) prettyPrint(r *response) string {
	h := r.Host
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	headers := []string{"Slug", "State", "Size", "IP", "Hostname", "Region"}
	values := []interface{}{h.Slug, h.State, h.Plan.Name, h.IPAddress, h.Hostname, h.Region.Name}

	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	fmt.Fprint(w, strings.Join(headers, "\t"), "\n")
	fmt.Fprint(w, insertAfterEach(values, "\t")...)
	fmt.Fprintln(w)

	stats := h.Stats
	if stats != nil {
		headers = []string{
			"Uptime", "Memory Used (bytes)", "Memory Total (bytes)",
			"Storage Used (bytes)", "Storage Total (bytes)",
			"Number of CPUs",
		}
		values = []interface{}{
			stats.Uptime, stats.MemoryUsage, stats.MemoryTotal,
			stats.StorageUsage, stats.StorageTotal,
			stats.NumCPU,
		}
		for i, usage := range stats.PerCPUUsage {
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
