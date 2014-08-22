package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
)

const DBEnvVar = "NITROUS_PG_URL"
const DigitalOceanImageID = "5714430"

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

type SeedFn func(db.QueryReceiver) error

var seeds = []SeedFn{users, regions, plans, templates}

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
		{Slug: "sfo", Rank: 100, Name: "US West (sfo)"},
		{Slug: "nyc", Rank: 200, Name: "US East (nyc)"},
		{Slug: "lon", Rank: 300, Name: "Europe (lon)"},
		{Slug: "ams", Rank: 400, Name: "Europe (ams)"},
		{Slug: "sgp", Rank: 500, Name: "Asia (sgp)"},
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
		{Slug: "512mb", CPU: "1", Memory: "512 MB", Storage: "20 GB", Name: "Nano", MaxContainers: 2, Price: 995},
		{Slug: "1gb", CPU: "1", Memory: "1 GB", Storage: "30 GB", Name: "Micro", MaxContainers: 4, Price: 1995},
		{Slug: "2gb", CPU: "2", Memory: "2 GB", Storage: "40 GB", Name: "Small", MaxContainers: 8, Price: 3995},
		{Slug: "4gb", CPU: "2", Memory: "4 GB", Storage: "60 GB", Name: "Medium", MaxContainers: 16, Price: 7995},
		{Slug: "8gb", CPU: "4", Memory: "8 GB", Storage: "80 GB", Name: "Large", MaxContainers: 32, Price: 15995},
		{Slug: "16gb", CPU: "8", Memory: "16 GB", Storage: "160 GB", Name: "XLarge", MaxContainers: 64, Price: 31995},
		{Slug: "32gb", CPU: "12", Memory: "32 GB", Storage: "320 GB", Name: "2XLarge", MaxContainers: 128, Price: 63995},
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
			providers := []string{"docker", "digitalocean"}
			for _, provider := range providers {
				pt := &providertemplate.ProviderTemplate{
					Name:     fmt.Sprintf("%s-%s-%s", provider, p.Slug, r.Slug),
					Provider: provider,
					RegionID: r.ID,
					PlanID:   p.ID,
					Config: hstore.Hstore{
						Map: map[string]sql.NullString{
							"region_id": {r.Slug, true},
							"size_id":   {p.Slug, true},
						},
					},
				}
				if provider == "digitalocean" {
					pt.Config.Map["image_id"] = sql.NullString{DigitalOceanImageID, true}
				}
				err := pt.Insert(d)
				if err != nil {
					return err
				}
				logr.Infof("Added provider template %+v", pt)
			}
		}
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
