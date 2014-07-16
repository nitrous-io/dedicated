package containers

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
)

type Container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
}

func PrettyPrint(headers []string, containers []Container) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	w.Init(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t "))
	for _, r := range containers {
		fmt.Fprintln(w,
			r.Name, "\t",
			r.State,
		)
	}
	w.Flush()
	return b.String()
}
