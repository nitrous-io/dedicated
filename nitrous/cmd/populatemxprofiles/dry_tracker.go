package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

type DryTracker struct{}

func (dt *DryTracker) TrackEvent(event string, u *user.User, p map[string]interface{}) {
	// no-op, fulfill interface
}

func (dt *DryTracker) CreateProfile(u *user.User, distinctID string) chan struct{} {
	// no-op, fulfill interface
	return nil
}

func (dt *DryTracker) SetProfileProps(mode string, userID int64, p map[string]interface{}) chan struct{} {
	logrus.Debugf("SetProfileProps: mode %s, userID, %d, props: %+v", mode, userID, p)
	return nil
}
