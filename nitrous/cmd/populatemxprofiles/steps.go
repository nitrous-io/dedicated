package main

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func SetConfirmed(jobs chan job, u *user.User) error {
	if u.ConfirmedAt.Valid {
		jobs <- job{mode: "setnx", uid: u.ID, p: map[string]interface{}{
			"confirmed_email_date": u.ConfirmedAt.Time,
		}}
	} else {
		logr.Debugf("No confirmation for user %s", u.Email)
	}
	return nil
}

func SetTemplates(jobs chan job, u *user.User) error {
	var images, allImages []string
	err := dbConn.Select(&allImages, `
		SELECT COALESCE(templates.slug, containers.image) FROM containers
		LEFT JOIN templates ON containers.image = templates.image
		WHERE user_id = $1 ORDER BY containers.created_at;
	`, u.ID)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, img := range allImages {
		if _, ok := seen[img]; ok {
			continue
		}
		seen[img] = struct{}{}
		images = append(images, img)
	}
	if len(images) > 0 {
		jobs <- job{mode: "set", uid: u.ID, p: map[string]interface{}{
			"images_list": images}}
	} else {
		logr.Debugf("No containers for user %s", u.Email)
	}
	return nil
}
