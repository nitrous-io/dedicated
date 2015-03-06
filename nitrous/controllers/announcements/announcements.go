package announcements

import (
	"encoding/json"
	"net/http"
	"time"

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
	errMultiSlugsNotSupported = httperror.New(http.StatusBadRequest,
		"multi_slugs_not_supported", "Sorry, only support single slug for now")
)

func showGlobal(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindAllForGlobal(env.DB)
	if err != nil {
		return err
	}

	serialized := serializers.SerializeAnnouncements(announcements)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"announcements": serialized,
	}

	return nil
}

func showAll(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindAll(env.DB)
	if err != nil {
		return err
	}

	serialized := serializers.SerializeHostAnnouncements(announcements)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"announcements": serialized,
	}

	return nil
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

	ann := &announcement.Announcement{
		SubjectTemplate: form.SubjectTemplate,
		MessageTemplate: form.MessageTemplate,
		ShowToAll:       form.ShowToAll,
		ExpiresAt:       form.ExpiresAt,
	}

	if form.ShowToAll {
		err := announcement.SendAndSaveGlobalAnnouncement(env.DB, ann)
		if err != nil {
			return err
		}

		env.Status = http.StatusAccepted
		env.Data = map[string]interface{}{
			"announcement": serializers.SerializeAnnouncement(ann),
		}

		return nil

	} else {
		slugs := form.HostSlugs

		// TODO: Only support single host slug for now
		// Because can't figure how to select and insert
		// multiple values to db :(
		if len(slugs) != 1 {
			return errMultiSlugsNotSupported
		}

		slug := slugs[0]

		owner, err := announcement.FindOwnerByHostSlug(env.DB, slug)
		if err != nil {
			return err
		}

		err = announcement.InsertForHost(env.DB, ann, owner.HostID)
		if err != nil {
			return err
		}

		err = announcement.SendToOwner(ann, owner)
		if err != nil {
			return err
		}

		hostAnn := &announcement.HostAnnouncement{
			Announcement: ann,
			HostOwner:    owner,
		}

		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"announcement": serializers.SerializeHostAnnouncement(hostAnn),
		}

		return nil
	}
}
