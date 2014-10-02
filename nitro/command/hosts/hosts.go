package hosts

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/nitrous-io/nitro/consts"
)

type Host struct {
	Slug      string `json:"slug"`
	State     string `json:"state"`
	Size      string `json:"size"`
	IPAddress string `json:"ip_address"`
}

func PrettyPrint(headers []string, hosts ...*Host) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	fmt.Fprintln(w, strings.Join(headers, "\t "))
	for _, h := range hosts {
		fmt.Fprintln(w, h.Slug, "\t", h.State, "\t", h.Size, "\t", h.IPAddress)
	}
	w.Flush()
	return b.String()
}
