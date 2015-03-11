package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
)

type Announcement struct {
	ID              int64      `json:"id"`
	SubjectTemplate string     `json:"subject_template"`
	MessageTemplate string     `json:"message_template"`
	ShowToAll       bool       `json:"show_to_all"`
	ExpiresAt       *time.Time `json:"expires_at"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

type HostOwner struct {
	HostID   int64  `json:"host_id"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Slug     string `json:"slug"`
}

type HostAnnouncement struct {
	*Announcement
	*HostOwner
	Subject *string `json:"subject"`
	Message *string `json:"message"`
}

func SerializeAnnouncement(ann *announcement.Announcement) *Announcement {
	if ann == nil {
		return nil
	}

	serialized := &Announcement{
		ID:              ann.ID,
		SubjectTemplate: ann.SubjectTemplate,
		MessageTemplate: ann.MessageTemplate,
		ShowToAll:       ann.ShowToAll,
	}

	if ann.ExpiresAt.Valid {
		serialized.ExpiresAt = &ann.ExpiresAt.Time
	}

	if ann.CreatedAt.Valid {
		serialized.CreatedAt = &ann.CreatedAt.Time
	}

	if ann.UpdatedAt.Valid {
		serialized.UpdatedAt = &ann.UpdatedAt.Time
	}

	return serialized
}

func SerializeHostOwner(owner *announcement.HostOwner) *HostOwner {
	if owner == nil {
		return nil
	}

	return &HostOwner{
		HostID:   owner.HostID,
		UserID:   owner.UserID,
		Username: owner.Username,
		Slug:     owner.Slug,
	}
}

func SerializeHostAnnouncement(ann *announcement.HostAnnouncement) *HostAnnouncement {
	if ann == nil {
		return nil
	}

	res := &HostAnnouncement{
		Announcement: SerializeAnnouncement(ann.Announcement),
		HostOwner:    SerializeHostOwner(ann.HostOwner),
	}

	if ann.HostOwner != nil {
		subject, err := ann.Subject()
		if err == nil {
			res.Subject = &subject
		}

		message, err := ann.Message()
		if err == nil {
			res.Message = &message
		}
	}

	return res
}

func SerializeHostOwners(owners []*announcement.HostOwner) []*HostOwner {
	res := make([]*HostOwner, len(owners))

	for i, owner := range owners {
		res[i] = SerializeHostOwner(owner)
	}

	return res
}

func SerializeAnnouncements(announcements []*announcement.Announcement) []*Announcement {
	res := make([]*Announcement, len(announcements))

	for i, ann := range announcements {
		res[i] = SerializeAnnouncement(ann)
	}

	return res
}

func SerializeHostAnnouncements(announcements []*announcement.HostAnnouncement) []*HostAnnouncement {
	res := make([]*HostAnnouncement, len(announcements))

	for i, ann := range announcements {
		res[i] = SerializeHostAnnouncement(ann)
	}

	return res
}
