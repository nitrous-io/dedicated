package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/containercollaborator"
)

type ContainerCollaboratorUser struct {
	ID          int64      `json:"id"`
	ContainerID int64      `json:"container_id"`
	UserID      int64      `json:"user_id"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`

	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

func SerializeContainerCollaboratorUser(sc *containercollaborator.ContainerCollaboratorUser) *ContainerCollaboratorUser {
	if sc == nil {
		return nil
	}
	return &ContainerCollaboratorUser{
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

func SerializeContainerCollaboratorUsers(scs []*containercollaborator.ContainerCollaboratorUser) []*ContainerCollaboratorUser {
	ret := make([]*ContainerCollaboratorUser, len(scs))
	for i, sc := range scs {
		ret[i] = SerializeContainerCollaboratorUser(sc)
	}
	return ret
}
