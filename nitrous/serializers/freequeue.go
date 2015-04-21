package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/freequeue"
)

type FreeQueueWithUser struct {
	ID     int64 `json:"id"`
	UserID int64 `json:"user_id"`

	State string `json:"state"`

	ContainerTemplateSlug string `json:"container_template_slug"`
	ContainerName         string `json:"container_name"`
	HostName              string `json:"host_name"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`

	Username         string `json:"username"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Phone            string `json:"phone"`
	PhoneCountryCode string `json:"phone_country_code"`
	PhoneAuthed      bool   `json:"phone_authed"`
}

func SerializeFreeQueueWithUser(f *freequeue.FreeQueueWithUser) *FreeQueueWithUser {
	if f == nil {
		return nil
	}
	return &FreeQueueWithUser{
		ID:     f.ID,
		UserID: f.UserID,
		State:  f.State,
		ContainerTemplateSlug: f.ContainerTemplateSlug,
		ContainerName:         f.ContainerName,
		HostName:              f.HostName,
		CreatedAt:             f.CreatedAt,
		UpdatedAt:             f.UpdatedAt,
		Username:              f.Username,
		Name:                  f.Name,
		Email:                 f.Email,
		Phone:                 f.Phone.String,
		PhoneCountryCode:      f.PhoneCountryCode.String,
		PhoneAuthed:           f.PhoneAuthed,
	}
}

func SerializeFreeQueueWithUsers(fq []*freequeue.FreeQueueWithUser) []*FreeQueueWithUser {
	ret := make([]*FreeQueueWithUser, len(fq))
	for i, f := range fq {
		ret[i] = SerializeFreeQueueWithUser(f)
	}
	return ret
}
