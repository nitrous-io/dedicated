package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/image"
)

type Image struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	HostID      int64      `json:"host_id"`
	FreighterID string     `json:"freighter_id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Tag         string     `json:"tag"`
	Repo        string     `json:"repo"`
	IsSnapshot  bool       `json:"is_snapshot"`
	Stack       string     `json:"stack"`
	ActualSize  int64      `json:"actual_size"`
	VirtualSize int64      `json:"virtual_size"`
	BuiltAt     *time.Time `json:"built_at,omitempty"`

	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func SerializeImage(i *image.Image) *Image {
	if i == nil {
		return nil
	}
	return &Image{
		ID:          i.ID,
		UserID:      i.UserID,
		HostID:      i.HostID,
		FreighterID: i.FreighterID,
		Name:        i.Name,
		Slug:        i.Slug,
		Tag:         i.Tag,
		IsSnapshot:  i.IsSnapshot,
		Stack:       i.Stack(),
		Repo:        i.FullRepository(),
		ActualSize:  i.ActualSize,
		VirtualSize: i.VirtualSize,
		BuiltAt:     i.BuiltAt,
		CreatedAt:   i.CreatedAt,
		UpdatedAt:   i.UpdatedAt,
	}
}

func SerializeImages(imgs []*image.Image) []*Image {
	ret := make([]*Image, len(imgs))
	for i, img := range imgs {
		ret[i] = SerializeImage(img)
	}
	return ret
}
