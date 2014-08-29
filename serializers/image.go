package serializers

import (
	"time"

	"github.com/nitrous-io/nitrous/models/image"
)

type Image struct {
	ID          string `json:"id"`
	UserID      int64  `json:"user_id"`
	HostID      int64  `json:"host_id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Tag         string `json:"tag"`
	ActualSize  int64  `json:"actual_size"`
	VirtualSize int64  `json:"virtual_size"`

	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func SerializeImage(i *image.Image) *Image {
	if i == nil {
		return nil
	}
	return &Image{
		ID:          i.FreighterID, // Use Freighter ID as "id".
		UserID:      i.UserID,
		HostID:      i.HostID,
		Name:        i.Name,
		Slug:        i.Slug,
		Tag:         i.Tag,
		ActualSize:  i.ActualSize,
		VirtualSize: i.VirtualSize,
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
