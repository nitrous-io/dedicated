package main

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	"github.com/nitrous-io/nitrous-common/logger"
)

const DigitalOceanImageID = "5714430"

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

type SeedFn func(db.QueryReceiver) error

var seeds = []SeedFn{users, regions, plans, providertemplates, templates}

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}
	if err := dbConn.MigrateAll(); err != nil {
		logr.Infof("Migrations failed: %v", err)
	}
	defer dbConn.DB.Close()

	tx, err := dbConn.Beginx()
	if err != nil {
		logr.FatalExit(err)
	}

	for _, f := range seeds {
		if err := f(tx); err != nil {
			tx.Rollback()
			logr.FatalExit(err)
		}
	}

	if err := tx.Commit(); err != nil {
		logr.FatalExit(err)
	}
}

func users(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "users")
	if err != nil || !ok {
		return err
	}
	users := []user.User{
		{Username: "admin", Email: "admins@nitrous.io", Admin: true},
		{Username: "host-provisioner", Email: "eng@nitrous.io", Admin: false, ServiceAccount: true},
	}
	for _, u := range users {
		pass, err := securerandom.Base64(12)
		if err != nil {
			return err
		}
		u.Password = pass

		if err := u.Insert(d.(*sqlx.Tx)); err != nil {
			return err
		}

		// set the max hosts to a large value for seed users
		_, err = d.Exec(`UPDATE user_profiles SET max_hosts = 9223372036854775807 WHERE user_id = $1;`, u.ID)
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
		if err := r.Insert(d); err != nil {
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
		{Slug: "shift", Name: "Shift", MaxContainers: 2, Price: 2500, CPU: "1", Memory: "1 GB", Storage: "20 GB"},
		{Slug: "turbo", Name: "Turbo", MaxContainers: 5, Price: 5000, CPU: "2", Memory: "2 GB", Storage: "30 GB", MostPopular: true},
		{Slug: "nitro", Name: "Nitro", MaxContainers: 10, Price: 8000, CPU: "2", Memory: "4 GB", Storage: "50 GB"},
	}
	for _, p := range plans {
		if err := p.Insert(d); err != nil {
			return err
		}
		logr.Infof("Added plan %+v", p)
	}
	return nil
}

func providertemplates(d db.QueryReceiver) error {
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

func templates(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "templates")
	if err != nil || !ok {
		return err
	}

	railsCn := defaultContainer()
	railsCn.Image = "nitrousio/starter-rails:latest"
	railsConfigJSON, err := templateCnCfgJSON(railsCn)
	if err != nil {
		return err
	}

	laravelCn := defaultContainer()
	laravelCn.Image = "nitrousio/starter-laravel:latest"
	laravelConfigJSON, err := templateCnCfgJSON(laravelCn)
	if err != nil {
		return err
	}

	meanCn := defaultContainer()
	meanCn.Image = "nitrousio/starter-mean:latest"
	meanConfigJSON, err := templateCnCfgJSON(meanCn)
	if err != nil {
		return err
	}

	meteorCn := defaultContainer()
	meteorCn.Image = "nitrousio/starter-meteor:latest"
	meteorConfigJSON, err := templateCnCfgJSON(meteorCn)
	if err != nil {
		return err
	}

	djangoCn := defaultContainer()
	djangoCn.Image = "nitrousio/starter-django:latest"
	djangoConfigJSON, err := templateCnCfgJSON(djangoCn)
	if err != nil {
		return err
	}

	templates := []template.Template{
		{
			Slug:        "rails",
			Name:        sql.NullString{"Ruby on Rails", true},
			Description: sql.NullString{railsDesc, true},
			Config:      string(railsConfigJSON),
			Type:        template.TypeStack,
		},
		{
			Slug:        "laravel",
			Name:        sql.NullString{"Laravel", true},
			Config:      string(laravelConfigJSON),
			Description: sql.NullString{laravelDesc, true},
			Type:        template.TypeStack,
		},
		{
			Slug:        "mean",
			Name:        sql.NullString{"MEAN", true},
			Config:      string(meanConfigJSON),
			Description: sql.NullString{meanDesc, true},
			Type:        template.TypeStack,
		},
		{
			Slug:        "meteor",
			Name:        sql.NullString{"Meteor", true},
			Config:      string(meteorConfigJSON),
			Description: sql.NullString{meteorDesc, true},
			Type:        template.TypeStack,
		},
		{
			Slug:        "django",
			Name:        sql.NullString{"Django", true},
			Config:      string(djangoConfigJSON),
			Description: sql.NullString{djangoDesc, true},
			Type:        template.TypeStack,
		},
	}
	for _, t := range templates {
		if err := t.Insert(d); err != nil {
			return err
		}
		logr.Infof("Added template %+v", t)
	}

	goCfgJSON, err := baseTemplateConfigJSON("nitrousio/golang:latest")
	if err != nil {
		return err
	}
	nodeCfgJSON, err := baseTemplateConfigJSON("nitrousio/nodejs:latest")
	if err != nil {
		return err
	}
	phpCfgJSON, err := baseTemplateConfigJSON("nitrousio/php:latest")
	if err != nil {
		return err
	}
	pythonCfgJSON, err := baseTemplateConfigJSON("nitrousio/python:latest")
	if err != nil {
		return err
	}
	rubyCfgJSON, err := baseTemplateConfigJSON("nitrousio/ruby:latest")
	if err != nil {
		return err
	}
	dindCfgJSON, err := privilegedBaseTemplateConfigJSON("nitrousio/docker:latest")
	if err != nil {
		return err
	}
	vanillaCfgJSON, err := baseTemplateConfigJSON("nitrousio/ubuntu:latest")
	if err != nil {
		return err
	}

	baseTemplates := []template.Template{
		{Name: sql.NullString{"Go", true}, Config: string(goCfgJSON), Slug: "golang", Type: template.TypeBasic},
		{Name: sql.NullString{"Node.js", true}, Config: string(nodeCfgJSON), Slug: "nodejs", Type: template.TypeBasic},
		{Name: sql.NullString{"PHP", true}, Config: string(phpCfgJSON), Slug: "php", Type: template.TypeBasic},
		{Name: sql.NullString{"Python", true}, Config: string(pythonCfgJSON), Slug: "python", Type: template.TypeBasic},
		{Name: sql.NullString{"Ruby", true}, Config: string(rubyCfgJSON), Slug: "ruby", Type: template.TypeBasic},
		{Name: sql.NullString{"Ubuntu", true}, Config: string(vanillaCfgJSON), Slug: "ubuntu", Type: template.TypeBasic},
		{Name: sql.NullString{"Docker", true}, Config: string(dindCfgJSON), Slug: "docker", Type: template.TypeAdvanced},
	}
	for _, t := range baseTemplates {
		if err := t.Insert(d); err != nil {
			return err
		}
		logr.Infof("Added base image %+v", t)
	}

	return nil
}

func baseTemplateConfigJSON(repo string) ([]byte, error) {
	c := template.Container{
		Privileged: false,
		PublishAll: false,
		Env:        nil,
		Image:      repo,
		Ports:      []string{},
		Volumes:    nil,
	}

	return templateCnCfgJSON(c)
}

func privilegedBaseTemplateConfigJSON(repo string) ([]byte, error) {
	c := template.Container{
		Privileged: true,
		PublishAll: false,
		Env:        nil,
		Image:      repo,
		Ports:      []string{},
		Volumes:    nil,
	}

	return templateCnCfgJSON(c)
}

func templateCnCfgJSON(cn template.Container) ([]byte, error) {
	cfg := template.TemplateConfig{
		Containers: []template.Container{cn},
		Volumes:    nil,
	}
	return json.Marshal(cfg)
}

func defaultContainer() template.Container {
	return template.Container{
		Name:       "app",
		Slug:       "app",
		Privileged: false,
		PublishAll: false,
		Env:        nil,
		Ports:      []string{"public::3000"},
		Volumes:    nil,
	}
}

func creditTypes(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "credit_types")
	if err != nil || !ok {
		return err
	}

	cType := &credittype.CreditType{
		Name:         "Signup_Credit",
		Description:  "Awarded to users for signing up",
		Active:       true,
		DefaultValue: 500, // $5
	}
	if err := cType.Insert(d); err != nil {
		return err
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
		logr.Errorf("Unexpected count %v", c)
		return false, err
	}
}

// Template descriptions
const (
	railsDesc = `A baseline Ruby on Rails stack that uses PostgreSQL as the database backend.

This template contains:
•  Rails 4.1
•  Ruby 2.1
•  PostgreSQL 9.3
•  Ubuntu 14.04`

	laravelDesc = `A PHP stack using Laravel, the PHP framework for web artisans.

This template contains:
•  Laravel 4.2.12
•  PHP 5.5.9
•  PostgreSQL 9.3
•  Ubuntu 14.04`

	meanDesc = `A MEAN Javascript stack: MongoDB, Express, AngularJS, and Node.js.

This template contains:
•  MEAN 0.4.1
•  Node.js 0.10.33
•  MongoDB 2.6.6
•  Ubuntu 14.04`

	meteorDesc = `A Meteor stack.

This template contains:
•  Meteor 1.0.1
•  Ubuntu 14.04`

	djangoDesc = `A Django stack.

This template contains:
•  Django 1.7.1
•  Python 3.4.0
•  IPython 2.3.1
•  Gunicorn 19.1.1
•  Ubuntu 14.04`

	vanillaDesc = `A vanilla development environment.

This template contains Ubuntu 14.04 with useful developer tools preinstalled.`
)
