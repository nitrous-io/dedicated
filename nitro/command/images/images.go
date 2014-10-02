package images

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nitrous-io/dedicated/nitro/consts"
)

type Image struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	HostID      int64      `json:"host_id"`
	FreighterID string     `json:"freighter_id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
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
	fmt.Fprintln(w, strings.Join([]string{"Slug", "Tag", "ID", "Name", "Created", "Size", "Virtual Size"}, "\t "))
	for _, h := range images {
		fmt.Fprintln(w, h.Slug, "\t", h.Tag, "\t", h.ID, "\t", h.Name, "\t", h.CreatedAt, "\t", h.ActualSize, "\t", h.VirtualSize)
	}
	w.Flush()
	return b.String()
}
