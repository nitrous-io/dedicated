package containers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/nitrous-io/nitro/command"
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

var errSplitHostContainerFailed = errors.New("cannot split host and container from string")

// SplitHostContainer parses a string of the form "hostname/containername" or
// into host name and container name.
// When of the form "containername", host is assumed to be the default host.
func SplitHostContainer(hostcontainer string) (host, container string, err error) {
	arr := strings.Split(hostcontainer, "/")
	switch len(arr) {
	case 1:
		host = command.DefaultHost()
		if host == "" {
			err = errSplitHostContainerFailed
			return
		}
		container = arr[0]
		return
	case 2:
		host, container = arr[0], arr[1]
		return
	default:
		err = errSplitHostContainerFailed
		return
	}
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
