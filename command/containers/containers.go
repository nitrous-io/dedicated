package containers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/nitrous-io/nitro/consts"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

type Container struct {
	Slug  string `json:"slug"`
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
			c.Slug, "\t",
			c.Image, "\t",
			c.State,
		)
	}
	w.Flush()
	return b.String()
}

func ParseContainerResponse(res *http.Response) (*Container, error) {
	err := httpclient.CheckResponse(res, http.StatusOK)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	j := struct {
		Container *Container `json:"container"`
	}{}
	err = json.NewDecoder(res.Body).Decode(&j)
	if err != nil {
		return nil, err
	}

	return j.Container, nil
}
