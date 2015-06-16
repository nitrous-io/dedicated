package analytics

import (
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

type Tracker interface {
	TrackEvent(event string, u *user.User, p map[string]interface{})
	CreateProfile(u *user.User, distinctID string) chan struct{}

	SetProfileProps(mode string, userID int64, p map[string]interface{}) chan struct{}
}
