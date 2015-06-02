package analytics

import (
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/nitrous-common/logger"
)

type Tracker interface {
	TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger)
	CreateProfile(u *user.User, logger *logger.Logger, distinctID string) chan struct{}

	SetProfileProps(mode string, userID int64, p map[string]interface{}, logger *logger.Logger) chan struct{}
}
