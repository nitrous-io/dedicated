package analytics

import (
	"strconv"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/go-mixpanel"
	"github.com/nitrous-io/nitrous-common/logger"
)

var (
	mixpanelToken string

	disabled = false
)

func init() {
	mixpanelToken = envvars.MixpanelToken
	if mixpanelToken == "" || strings.HasPrefix(mixpanelToken, "disable") {
		disabled = true
	}
}

type MixpanelTracker struct {
	client *mixpanel.Mixpanel
}

func NewMixpanelTracker() *MixpanelTracker {
	var mc *mixpanel.Mixpanel = nil

	if !disabled {
		mc = mixpanel.NewMixpanelClient(mixpanelToken)
	}

	return &MixpanelTracker{client: mc}
}

func (m *MixpanelTracker) TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger) {
	if disabled {
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

func (m *MixpanelTracker) CreateProfile(u *user.User, logger *logger.Logger, inviteCode string) {
	if disabled {
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
	if inviteCode != "" {
		p["invite_code"] = inviteCode
	}

	go func() {
		err := m.client.CreateProfile(strconv.Itoa(int(u.ID)), p)
		if err != nil {
			logger.Errorf("analytics: failed to create user profile: %s. err: %v", u.Email, err)
		}
	}()
}
