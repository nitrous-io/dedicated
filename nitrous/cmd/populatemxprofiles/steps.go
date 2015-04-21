package main

func GenConfirmedJobs(jobs chan job, u *User) error {
	if u.ConfirmedAt.Valid {
		jobs <- NewJob("setnx", u.ID, map[string]interface{}{
			"confirmed_email_date": u.ConfirmedAt.Time,
		})
	} else {
		logr.Debugf("No confirmation for user %s", u.Email)
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
		logr.Debugf("No containers for user %s", u.Email)
	}
	return nil
}

func GenGithubAuthorizedJobs(jobs chan job, u *User) error {
	if !u.GithubUsername.Valid {
		return nil
	}
	if !u.CreatedAt.Valid {
		logr.Debugf("Unexpected nil created at for user %s", u.Email)
		return nil
	}
	jobs <- NewJob("setnx", u.ID, map[string]interface{}{
		"github_username":        u.GithubUsername.String,
		"github_authorized_date": u.CreatedAt.Time, // no date stored, use created_at as proxy
	})
	return nil
}
