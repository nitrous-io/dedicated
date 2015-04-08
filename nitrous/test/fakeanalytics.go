package test

import (
	"github.com/nitrous-io/dedicated/nitrous/models/user"

	"github.com/nitrous-io/nitrous-common/logger"
)

type FakeTracker struct {
	Events               map[int64][]string
	TrackEventProperties map[int64][]map[string]interface{}
	ProfileCreated       *user.User
	PropsCalled          []*PropsCall
}

type PropsCall struct {
	UserID   int64
	Mode     string
	PropsSet map[string]interface{}
	PropsInc map[string]int
}

func NewFakeTracker() *FakeTracker {
	return &FakeTracker{
		Events:               map[int64][]string{},
		TrackEventProperties: map[int64][]map[string]interface{}{},
	}
}

func (f *FakeTracker) TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger) {
	if u == nil {
		return
	}

	if f.Events[u.ID] == nil {
		f.Events = map[int64][]string{u.ID: []string{event}}
	} else {
		f.Events[u.ID] = append(f.Events[u.ID], event)
	}

	if f.TrackEventProperties[u.ID] == nil {
		f.TrackEventProperties = map[int64][]map[string]interface{}{u.ID: []map[string]interface{}{p}}
	} else {
		f.TrackEventProperties[u.ID] = append(f.TrackEventProperties[u.ID], p)
	}
}

func (f *FakeTracker) CreateProfile(u *user.User, logger *logger.Logger) {
	f.ProfileCreated = u
}

func (f *FakeTracker) SetProfileProps(mode string, userID int64, p map[string]interface{}, logger *logger.Logger) {
	f.PropsCalled = append(f.PropsCalled, &PropsCall{
		Mode:     mode,
		UserID:   userID,
		PropsSet: p,
	})
}

func (f *FakeTracker) IncProfileProps(userID int64, p map[string]int, logger *logger.Logger) {
	f.PropsCalled = append(f.PropsCalled, &PropsCall{
		UserID:   userID,
		PropsInc: p,
	})
}
