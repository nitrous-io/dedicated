package images

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nitrous-io/nitro/consts"
)

var headers = []string{"ID", "Name", "Tag", "Created", "Size", "Virtual Size"}

type Image struct {
	ID          string     `json:"id"`
	UserID      int64      `json:"user_id"`
	HostID      int64      `json:"host_id"`
	Name        string     `json:"name"`
	Tag         string     `json:"tag"`
	ActualSize  int64      `json:"actual_size"`
	VirtualSize int64      `json:"virtual_size"`
	BuiltAt     *time.Time `json:"built_at"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

func PrettyPrint(images ...*Image) string {
	b := &bytes.Buffer{}
	w := &tabwriter.Writer{}

	w.Init(b, consts.MinWidth, consts.TabWidth, consts.Padding, consts.PaddingChar, consts.Flags)
	fmt.Fprintln(w, strings.Join(headers, "\t "))
	for _, h := range images {
		fmt.Fprintln(w, h.ID, "\t", h.Name, "\t", h.Tag, "\t", h.CreatedAt, "\t", h.ActualSize, "\t", h.VirtualSize)
	}
	w.Flush()
	return b.String()
}
