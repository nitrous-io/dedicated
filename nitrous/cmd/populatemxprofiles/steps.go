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
