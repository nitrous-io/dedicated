package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
)

const DBEnvVar = "NITROUS_PG_URL"

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

type SeedFn func(db.QueryReceiver) error

var seeds = []SeedFn{users, regions, plans, templates, hosts, containers}

func main() {
	dbURL := os.Getenv(DBEnvVar)
	if dbURL == "" {
		logr.FatalExit(DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}
	err = dbConn.MigrateAll()
	if err != nil {
		logr.Infof("Migrations failed: %v", err)
	}
	defer dbConn.DB.Close()

	tx, err := dbConn.Beginx()
	if err != nil {
		logr.FatalExit(err)
	}

	for _, f := range seeds {
		err := f(tx)
		if err != nil {
			tx.Rollback()
			logr.FatalExit(err)
		}
	}

	err = tx.Commit()
	if err != nil {
		logr.FatalExit(err)
	}
}

func users(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "users")
	if err != nil || !ok {
		return err
	}
	users := []user.User{
		{Username: "seeduser", Email: "seeduser@example.com", Password: "123456"},
	}
	for _, u := range users {
		err = u.Insert(d)
		if err != nil {
			return err
		}
		logr.Infof("Added user %+v", u)
	}
	return nil
}

func regions(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "regions")
	if err != nil || !ok {
		return err
	}
	regions := []region.Region{
		{Name: "apse", Description: "Southeast Asia"},
		{Name: "use", Description: "US East"},
		{Name: "usw", Description: "US West"},
		{Name: "euw", Description: "Europe"},
	}
	for _, r := range regions {
		err = r.Insert(d)
		if err != nil {
			return err
		}
		logr.Infof("Added region %+v", r)
	}
	return nil
}

func plans(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "plans")
	if err != nil || !ok {
		return err
	}
	plans := []plan.Plan{
		{Name: "nano", MaxContainers: 1, Price: 500},
		{Name: "micro", MaxContainers: 2, Price: 1000},
		{Name: "small", MaxContainers: 3, Price: 1500},
		{Name: "medium", MaxContainers: 4, Price: 2000},
		{Name: "large", MaxContainers: 5, Price: 2500},
	}
	for _, p := range plans {
		err = p.Insert(d)
		if err != nil {
			return err
		}
		logr.Infof("Added plan %+v", p)
	}
	return nil
}

func templates(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "provider_templates")
	if err != nil || !ok {
		return err
	}
	plans := []plan.Plan{}
	err = d.Select(&plans, "SELECT * FROM plans")
	if err != nil {
		return err
	}
	regions := []region.Region{}
	err = d.Select(&regions, "SELECT * FROM regions")
	if err != nil {
		return err
	}
	for _, p := range plans {
		for _, r := range regions {
			pt := &providertemplate.ProviderTemplate{
				Name:     fmt.Sprintf("docker-%s-%s", p.Name, r.Name),
				Provider: "docker",
				RegionID: r.ID,
				PlanID:   p.ID,
				Config: hstore.Hstore{
					Map: map[string]sql.NullString{
						"image": {"nitrousio/ubuntu-dind", true},
					},
				},
			}
			err := pt.Insert(d)
			if err != nil {
				return err
			}
			logr.Infof("Added provider template %+v", pt)
		}
	}
	return nil
}

func hosts(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "hosts")
	if err != nil || !ok {
		return err
	}
	pts := []providertemplate.ProviderTemplate{}
	err = d.Select(&pts, "SELECT * FROM provider_templates")
	if err != nil {
		return err
	}
	users := []user.User{}
	err = d.Select(&users, "SELECT * FROM users")
	if err != nil {
		return err
	}
	for _, pt := range pts {
		for _, u := range users {
			h := &host.Host{
				Name:               fmt.Sprintf("host-%d", pt.ID),
				RegionID:           pt.RegionID,
				PlanID:             pt.PlanID,
				ProviderTemplateID: pt.ID,
				UserID:             u.ID,
			}
			err := h.Insert(d)
			if err != nil {
				return err
			}
			logr.Infof("Added host %+v", h)
		}
	}
	return nil
}

func containers(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "containers")
	if err != nil || !ok {
		return err
	}
	hs := []host.Host{}
	err = d.Select(&hs, "SELECT * FROM hosts")
	if err != nil {
		return err
	}
	for _, h := range hs {
		c := &container.Container{
			ID:     fmt.Sprintf("cc%dff%d", h.ID, h.UserID),
			UserID: h.UserID,
			HostID: h.ID,
			Image:  "nitrous-base",
			Name:   fmt.Sprintf("%s-container-%d", h.Name, h.UserID),
		}
		err := c.Insert(d)
		if err != nil {
			return err
		}
		logr.Infof("Added container %+v", c)
	}
	return nil
}

// shouldSeed takes a tableName and returns true if it has zero rows
func shouldSeed(d db.QueryReceiver, tableName string) (bool, error) {
	c := -1
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id IS NOT NULL", tableName)
	err := d.QueryRowx(query).Scan(&c)
	switch {
	case err != nil:
		return false, err
	case c > 0:
		logr.Infof("%d %s found, skipping %s seeds", c, tableName, tableName)
		return false, nil
	case c == 0:
		return true, nil
	default: // if it's still -1
		logr.Errorf("Unexpected count %s", c)
		return false, err
	}
}
