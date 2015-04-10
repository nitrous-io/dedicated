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
		err := m.client.NewProfileOp(strconv.Itoa(int(u.ID)), p).Create()
		if err != nil {
			logger.Errorf("analytics: failed to create user profile: %s. err: %v", u.Email, err)
		}
	}()
}

func (m *MixpanelTracker) SetProfileProps(mode string, userID int64, p map[string]interface{}, logger *logger.Logger) chan struct{} {
	var propFn func() error
	uid := strconv.Itoa(int(userID))

	if ip, ok := p["$ip"]; !ok || ip == "" {
		// do not use request address as IP (mixpanel default if $ip missing)
		p["$ip"] = "0"
	}

	op := m.client.NewProfileOp(uid, p)

	switch mode {
	case "set":
		propFn = op.Set
	case "setnx": // set not exist
		propFn = op.SetOnce
	case "append":
		propFn = op.Append
	case "inc":
		propFn = op.Increment
	default:
		logger.Errorf("analytics: unsupported mode given: %s", mode)
		return nil
	}

	finished := make(chan struct{}, 1)
	go func() {
		err := propFn()
		if err != nil {
			logger.Errorf("analytics: failed to update (%s) user profile: %s. err: %v", mode, uid, err)
		}
		finished <- struct{}{}
	}()
	return finished
}
