package announcements

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	ShowGlobal       = admin.VerifyAdmin(showGlobal)
	ShowAll          = admin.VerifyAdmin(showAll)
	SendAnnouncement = admin.VerifyAdmin(sendAnnouncement)

	errJSONFormRequired = httperror.New(http.StatusBadRequest,
		"json_form_required", "POST content-type must be application/json")

	errInvalidJSON = httperror.New(http.StatusBadRequest,
		"invalid_json", "Invalid JSON")

	errNoSlugSpecified = httperror.New(http.StatusBadRequest,
		"no_slug_specified", "Must specify at least one host slug")

	errEmptySubject = httperror.New(http.StatusBadRequest,
		"empty_subject", "Subject field is empty")

	errEmptyMessage = httperror.New(http.StatusBadRequest,
		"message_subject", "Message field is empty")

	errInvalidExpiry = httperror.New(http.StatusBadRequest,
		"invalid_expiry", "Invalid announcement expiry date")

	errSlugsNotFound = httperror.New(http.StatusNotFound,
		"slugs_not_found", "Some of the host slugs cannot be found")
)

func showGlobal(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindAllForGlobal(env.DB)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"announcements": serializers.SerializeAnnouncements(announcements),
	}

	return nil
}

func showAll(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindAll(env.DB)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"announcements": serializers.SerializeHostAnnouncements(announcements),
	}

	return nil
}

func mailErrorLogger() func(error) {
	return func(err error) {
		logrus.Warnf("Failed to send mail to host owners: %s", err)
	}
}

func sendAnnouncement(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	var announcementForm struct {
		Content struct {
			SubjectTemplate string     `json:"subject_template"`
			MessageTemplate string     `json:"message_template"`
			ShowToAll       bool       `json:"show_to_all"`
			ExpiresAt       *time.Time `json:"expires_at"`
			HostSlugs       []string   `json:"slugs"`
		} `json:"announcement"`
	}

	if r.Header.Get("Content-Type") != "application/json" {
		return errJSONFormRequired
	}

	err := json.NewDecoder(r.Body).Decode(&announcementForm)
	if err != nil {
		return errInvalidJSON
	}

	form := announcementForm.Content

	if form.SubjectTemplate == "" {
		return errEmptySubject
	}

	if form.MessageTemplate == "" {
		return errEmptyMessage
	}

	if form.ExpiresAt == nil || form.ExpiresAt.Before(time.Now()) {
		return errInvalidExpiry
	}

	ann := &announcement.Announcement{
		SubjectTemplate: form.SubjectTemplate,
		MessageTemplate: form.MessageTemplate,
		ShowToAll:       form.ShowToAll,
		ExpiresAt:       pq.NullTime{*form.ExpiresAt, true},
	}

	errHandler := mailErrorLogger()

	if form.ShowToAll {
		err := announcement.SendAndSaveGlobalAnnouncement(env.DB, ann, errHandler)
		if err != nil {
			return err
		}

		env.Status = http.StatusAccepted
		env.Data = map[string]interface{}{
			"announcements": serializers.SerializeAnnouncements([]*announcement.Announcement{ann}),
		}

		return nil

	} else {
		slugs := form.HostSlugs
		if len(slugs) == 0 {
			return errNoSlugSpecified
		}

		hostAnns, err := announcement.SendAndSaveForHostSlugs(env.DB, ann, form.HostSlugs, errHandler)
		if err != nil {
			if err == announcement.ErrSlugsNotFound {
				return errSlugsNotFound
			}

			return err
		}

		env.Status = http.StatusAccepted
		env.Data = map[string]interface{}{
			"announcements": serializers.SerializeHostAnnouncements(hostAnns),
		}

		return nil
	}
}
