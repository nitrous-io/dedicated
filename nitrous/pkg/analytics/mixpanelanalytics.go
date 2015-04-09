package analytics

import (
	"strconv"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/go-mixpanel"
	"github.com/nitrous-io/nitrous-common/logger"
)

type MixpanelTracker struct {
	client *mixpanel.Mixpanel
}

func NewMixpanelTracker(token string) *MixpanelTracker {
	var mc *mixpanel.Mixpanel = nil

	if !(token == "" || strings.HasPrefix(token, "disable")) {
		mc = mixpanel.NewMixpanelClient(token)
	}

	return &MixpanelTracker{client: mc}
}

func (m *MixpanelTracker) TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger) {
	if m.client == nil {
		return
	}

	if p == nil {
		p = map[string]interface{}{}
	}

	p["distinct_id"] = strconv.Itoa(int(u.ID))
	p["email"] = u.Email
	p["admin"] = u.Admin

	go func() {
		err := m.client.Track(event, p)
		if err != nil {
			logger.Errorf("analytics: failed to track event: %s. err: %v", event, err)
		}
	}()
}

func (m *MixpanelTracker) CreateProfile(u *user.User, logger *logger.Logger) {
	if m.client == nil {
		return
	}

	p := map[string]interface{}{
		"$email":    u.Email,
		"$name":     u.Name,
		"$username": u.Username,
		"$created":  u.CreatedAt,
		"user_id":   u.ID,
		"admin":     u.Admin,
	}

	go func() {
		err := m.client.CreateProfile(strconv.Itoa(int(u.ID)), p)
		if err != nil {
			logger.Errorf("analytics: failed to create user profile: %s. err: %v", u.Email, err)
		}
	}()
}
