package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
)

type ContainerCollabUser struct {
	ID          int64      `json:"id"`
	ContainerID int64      `json:"container_id"`
	UserID      int64      `json:"user_id"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`

	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

func SerializeContainerCollabUser(sc *containercollab.ContainerCollabUser) *ContainerCollabUser {
	if sc == nil {
		return nil
	}
	return &ContainerCollabUser{
		ID:          sc.ID,
		ContainerID: sc.ContainerID,
		UserID:      sc.UserID,
		CreatedAt:   sc.CreatedAt,
		UpdatedAt:   sc.UpdatedAt,
		Username:    sc.Username,
		Email:       sc.Email,
		Name:        sc.Name,
	}
}

func SerializeContainerCollabUsers(scs []*containercollab.ContainerCollabUser) []*ContainerCollabUser {
	ret := make([]*ContainerCollabUser, len(scs))
	for i, sc := range scs {
		ret[i] = SerializeContainerCollabUser(sc)
	}
	return ret
}
