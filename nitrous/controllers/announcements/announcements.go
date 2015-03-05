package announcements

import (
	"net/http"

	dbLib "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
)

func ShowGlobalAnnouncements(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindForGlobal(env.DB)
	if err != nil {
		return err
	}

	env.Logger.Infof("global announcements: %+v", announcements)

	env.Status = http.StatusOK
	return nil
}

func SendAndSaveGlobalAnnouncement(db *dbLib.Database, ann *announcement.Announcement) error {
	owners, err := announcement.FindAllOwners(db)
	if err != nil {
		return err
	}

	err = announcement.InsertForGlobal(db, ann)
	if err != nil {
		return err
	}

	// ignore error for now
	announcement.SendToOwners(ann, owners, func(error) {})

	return nil
}
