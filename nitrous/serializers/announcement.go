package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
)

type HostAnnouncement struct {
	ID        int64      `json:"id"`
	HostID    int64      `json:"host_id"`
	UserID    int64      `json:"user_id"`
	Username  string     `json"username"`
	Slug      string     `json:"slug"`
	IsGlobal  bool       `json:"is_global"`
	Subject   string     `json:subject`
	Message   string     `json:message`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeHostAnnouncement(ann *announcement.HostAnnouncement) (*HostAnnouncement, error) {
	if ann == nil {
		return nil, nil
	}

	subject, err := ann.Subject()
	if err != nil {
		return nil, err
	}

	message, err := ann.Message()
	if err != nil {
		return nil, err
	}

	serialized := &HostAnnouncement{
		ID:        ann.ID,
		HostID:    ann.HostID,
		UserID:    ann.UserID,
		Username:  ann.Username,
		Slug:      ann.Slug,
		IsGlobal:  ann.IsGlobal(),
		Subject:   subject,
		Message:   message,
		ExpiresAt: ann.ExpiresAt,
		CreatedAt: ann.CreatedAt,
		UpdatedAt: ann.UpdatedAt,
	}

	return serialized, nil
}

func SerializeHostAnnouncements(announcements []*announcement.HostAnnouncement) ([]*HostAnnouncement, error) {
	res := make([]*HostAnnouncement, len(announcements))

	for i, ann := range announcements {
		ann, err := SerializeHostAnnouncement(ann)
		if err != nil {
			return nil, err
		}

		res[i] = ann
	}

	return res, nil
}
