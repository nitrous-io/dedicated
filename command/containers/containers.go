package containers

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/nitrous-io/nitro/consts"
)

type Container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
}

func PrettyPrint(headers []string, containers ...*Container) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	fmt.Fprintln(w, strings.Join(headers, "\t "))
	for _, c := range containers {
		fmt.Fprintln(w,
			c.Name, "\t",
			c.State,
		)
	}
	w.Flush()
	return b.String()
}
