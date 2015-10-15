package main

import (
	"database/sql"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func GenConfirmedJobs(jobs chan job, u *User) error {
	if u.ConfirmedAt.Valid {
		jobs <- NewJob("setnx", u.ID, map[string]interface{}{
			"confirmed_email_date": u.ConfirmedAt.Time,
		})
	} else {
		logrus.Debugf("No confirmation for user %s", u.Email)
	}
	return nil
}

func GenTemplatesJobs(jobs chan job, u *User) error {
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
		jobs <- NewJob("set", u.ID, map[string]interface{}{
			"images_list": images,
		})
	} else {
		logrus.Debugf("No containers for user %s", u.Email)
	}
	return nil
}

func GenGithubAuthorizedJobs(jobs chan job, u *User) error {
	if !u.GithubUsername.Valid {
		return nil
	}
	if !u.CreatedAt.Valid {
		logrus.Debugf("Unexpected nil created at for user %s", u.Email)
		return nil
	}
	jobs <- NewJob("setnx", u.ID, map[string]interface{}{
		"github_username":        u.GithubUsername.String,
		"github_authorized_date": u.CreatedAt.Time, // no date stored, use created_at as proxy
	})
	return nil
}

func GenPhoneJobs(jobs chan job, u *User) error {
	if !u.PhoneAuthed {
		// fix wrongly set verified date
		logrus.Debugf("No phone auth for user %s", u.Email)
		jobs <- NewJob("unset", u.ID, map[string]interface{}{
			"phone_number_verified_date": 0,
		})
		return nil
	}
	if u.Phone.String == "" || u.PhoneCountryCode.String == "" {
		logrus.Debugf("No phone number for user %s", u.Email)
		return nil
	}
	phone := fmt.Sprintf("+%s-%s", u.PhoneCountryCode.String, u.Phone.String)
	jobs <- NewJob("set", u.ID, map[string]interface{}{
		"phone_number": phone,
	})
	return nil
}

func GenHostCreatedJobs(jobs chan job, u *User) error {
	plans := []string{}
	// we assume that orphan/born etc state hosts will be subtracted from the count upon termination
	err := dbConn.Select(&plans, `
		SELECT plans.name FROM hosts
		JOIN plans on hosts.plan_id = plans.id
		WHERE terminated_at IS NULL
		AND user_id = $1
		ORDER BY hosts.created_at;
	`, u.ID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	jobs <- NewJob("set", u.ID, map[string]interface{}{
		"workspaces_plans": plans,
	})
	return nil
}

func GenMixpanelProfileJobs(jobs chan job, u *User) error {
	logrus.Infof("creating mixpanel profile for %s", u.Email)
	u2 := &user.User{
		ID:        u.ID,
		Name:      u.Name,
		Username:  u.Username,
		Email:     u.Email,
		CreatedAt: &u.CreatedAt.Time,
	}
	finished := tracker.CreateProfile(u2, "")
	if finished != nil {
		<-finished
	}
	return nil
}

func GenCollabInvitesJobs(jobs chan job, u *User) error {
	var invitesSent int
	err := dbConn.Get(&invitesSent, `
		SELECT COUNT(*) FROM container_collabs
		JOIN containers ON container_collabs.container_id = containers.id
		JOIN users ON containers.user_id = users.id
		WHERE containers.user_id = $1
	`, u.ID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	jobs <- NewJob("set", u.ID, map[string]interface{}{
		"collab_invites_count": invitesSent,
	})
	return nil
}

func GenUnsetDeletedJobs(jobs chan job) error {
	users := []*User{}
	err := dbConn.Select(&users, `
	SELECT email, MIN(users.id) AS user_id
	FROM users
	GROUP BY email
	HAVING COUNT(email) > 1
	ORDER BY user_id
	`)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, u := range users {
		jobs <- NewJob("unset", u.ID, map[string]interface{}{
			"deleted_date": 0,
		})
	}
	return nil
}
