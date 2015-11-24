package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
)

type ContainerCollabUser struct {
	ID          int64      `json:"id"`
	ContainerID int64      `json:"container_id"`
	UserID      int64      `json:"user_id,omitempty"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`

	Username     string `json:"username,omitempty"`
	Name         string `json:"name,omitempty"`
	GravatarHash string `json:"gravatar_hash,omitempty"`

	Owner bool `json:"owner,omitempty"`

	PendingEmail string `json:"pending_email,omitempty"`
}

func SerializeContainerCollabUser(sc *containercollab.ContainerCollabUser) *ContainerCollabUser {
	if sc == nil {
		return nil
	}

	ccu := &ContainerCollabUser{
		ID:          sc.ID,
		ContainerID: sc.ContainerID,
		CreatedAt:   sc.CreatedAt,
		UpdatedAt:   sc.UpdatedAt,
		Username:    sc.Username.String,
		Name:        sc.Name.String,
	}

	if sc.UserID.Valid {
		ccu.UserID = sc.UserID.Int64
	}
	if sc.InviteEmail.Valid {
		ccu.PendingEmail = sc.InviteEmail.String
	}
	// show the gravatar of either the joined user or
	// the email of the pending invite (who has not joined Nitrous)
	if sc.Email.Valid || sc.InviteEmail.Valid {
		str := sc.Email.String
		if str == "" {
			str = sc.InviteEmail.String
		}
		ccu.GravatarHash = generateGravatarHash(str)
	}

	return ccu
}

func SerializeContainerCollabUsers(scs []*containercollab.ContainerCollabUser) []*ContainerCollabUser {
	ret := make([]*ContainerCollabUser, len(scs))
	for i, sc := range scs {
		ret[i] = SerializeContainerCollabUser(sc)
	}
	return ret
}
