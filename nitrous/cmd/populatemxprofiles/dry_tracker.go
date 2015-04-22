package main

import (
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/nitrous-common/logger"
)

type DryTracker struct{}

func (dt *DryTracker) TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger) {
	// no-op, fulfill interface
}

func (dt *DryTracker) CreateProfile(u *user.User, logger *logger.Logger) {
	// no-op, fulfill interface
}

func (dt *DryTracker) SetProfileProps(mode string, userID int64, p map[string]interface{}, logger *logger.Logger) chan struct{} {
	logger.Debugf("SetProfileProps: mode %s, userID, %d, props: %+v", mode, userID, p)
	return nil
}
