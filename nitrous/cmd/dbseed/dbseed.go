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
		{Slug: "1gb", Name: "Shift", MaxContainers: 2, Price: 2500, CPU: "1", Memory: "1 GB", Storage: "20 GB"},
		{Slug: "2gb", Name: "Turbo", MaxContainers: 5, Price: 5000, CPU: "2", Memory: "2 GB", Storage: "30 GB", MostPopular: true},
		{Slug: "4gb", Name: "Nitro", MaxContainers: 10, Price: 8000, CPU: "2", Memory: "4 GB", Storage: "50 GB"},
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
	railsCfgJSON, err := templateCnCfgJSON(railsCn)
	if err != nil {
		return err
	}

	laravelCn := defaultContainer()
	laravelCn.Image = "nitrousio/starter-laravel:latest"
	laravelCfgJSON, err := templateCnCfgJSON(laravelCn)
	if err != nil {
		return err
	}

	meanCn := defaultContainer()
	meanCn.Image = "nitrousio/starter-mean:latest"
	meanCfgJSON, err := templateCnCfgJSON(meanCn)
	if err != nil {
		return err
	}

	meteorCn := defaultContainer()
	meteorCn.Image = "nitrousio/starter-meteor:latest"
	meteorCfgJSON, err := templateCnCfgJSON(meteorCn)
	if err != nil {
		return err
	}

	djangoCn := defaultContainer()
	djangoCn.Image = "nitrousio/starter-django:latest"
	djangoCfgJSON, err := templateCnCfgJSON(djangoCn)
	if err != nil {
		return err
	}

	templates := []template.Template{
		newTemplate("Ruby on Rails", "rails", railsCfgJSON, template.TypeStack, railsDesc),
		newTemplate("Laravel", "laravel", laravelCfgJSON, template.TypeStack, laravelDesc),
		newTemplate("MEAN", "mean", meanCfgJSON, template.TypeStack, laravelDesc),
		newTemplate("Meteor", "meteor", meteorCfgJSON, template.TypeStack, meteorDesc),
		newTemplate("Django", "django", djangoCfgJSON, template.TypeStack, djangoDesc),
	}
	for _, t := range templates {
		if err := t.Insert(d); err != nil {
			return err
		}
		logr.Infof("Added template %+v", t)
	}

	goCfgJSON, err := baseTemplateCfgJSON("nitrousio/golang:latest")
	if err != nil {
		return err
	}
	nodeCfgJSON, err := baseTemplateCfgJSON("nitrousio/nodejs:latest")
	if err != nil {
		return err
	}
	phpCfgJSON, err := baseTemplateCfgJSON("nitrousio/php:latest")
	if err != nil {
		return err
	}
	pythonCfgJSON, err := baseTemplateCfgJSON("nitrousio/python:latest")
	if err != nil {
		return err
	}
	rubyCfgJSON, err := baseTemplateCfgJSON("nitrousio/ruby:latest")
	if err != nil {
		return err
	}
	dindCfgJSON, err := privilegedBaseTemplateCfgJSON("nitrousio/docker:latest")
	if err != nil {
		return err
	}
	vanillaCfgJSON, err := baseTemplateCfgJSON("nitrousio/ubuntu:latest")
	if err != nil {
		return err
	}

	baseTemplates := []template.Template{
		newTemplate("Go", "golang", goCfgJSON, template.TypeBasic, goDesc),
		newTemplate("Node.js", "nodejs", nodeCfgJSON, template.TypeBasic, nodejsDesc),
		newTemplate("PHP", "php", phpCfgJSON, template.TypeBasic, phpDesc),
		newTemplate("Python", "python", pythonCfgJSON, template.TypeBasic, pythonDesc),
		newTemplate("Ruby", "ruby", rubyCfgJSON, template.TypeBasic, rubyDesc),
		newTemplate("Ubuntu", "ubuntu", vanillaCfgJSON, template.TypeBasic, vanillaDesc),
		newTemplate("Docker", "docker", dindCfgJSON, template.TypeAdvanced, dockerDesc),
	}
	for _, t := range baseTemplates {
		if err := t.Insert(d); err != nil {
			return err
		}
		logr.Infof("Added base image %+v", t)
	}

	return nil
}

func baseTemplateCfgJSON(repo string) ([]byte, error) {
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

func privilegedBaseTemplateCfgJSON(repo string) ([]byte, error) {
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

func newTemplate(name, slug string, cfgJSON []byte, tempType, desc string) template.Template {
	return template.Template{
		Name:        sql.NullString{name, true},
		Config:      string(cfgJSON),
		Slug:        slug,
		Type:        tempType,
		Description: sql.NullString{desc, true},
	}
}

// Template descriptions
const (
	goDesc = `Go: an open source programming language that makes it easy to build simple, reliable, and efficient software

•  Go 1.4
•  Ubuntu 14.04`

	phpDesc = `PHP: a popular general-purpose scripting language that is especially suited to web development

•  Composer 1.0.0-alpha9
•  PHP 5.5.9
•  Ubuntu 14.04`

	nodejsDesc = `Node.js: a platform built on Chrome's JavaScript runtime for easily building fast, scalable network applications

•  nvm 0.22.0
•  Node.js 0.10.35
•  Ubuntu 14.04`

	pythonDesc = `Python: a programming language that lets you work quickly and integrate systems more effectively

•  virtualenv 1.11.4
•  python 3.4.0
•  python 2.7.6
•  Ubuntu 14.04`

	rubyDesc = `Ruby: a dynamic, open source programming language with a focus on simplicity and productivity.

•  bundler 1.7.9
•  ruby 2.1.5
•  Ubuntu 14.04`

	dockerDesc = `A development environment with Docker set up.

•  Docker 1.4.1
•  Ubuntu 14.04`

	railsDesc = `Ruby on Rails: an open-source framework optimized for programmer happiness and sustainable productivity

•  Rails 4.1
•  Ruby 2.1.5
•  PostgreSQL 9.3
•  Ubuntu 14.04`

	laravelDesc = `Laravel, the PHP framework for web artisans

•  Laravel 4.2.12
•  PHP 5.5.9
•  PostgreSQL 9.3
•  Ubuntu 14.04`

	meanDesc = `MEAN: an opinionated full-stack JavaScript framework

•  MEAN 0.4.1
•  Node.js 0.10.33
•  MongoDB 2.6.6
•  Ubuntu 14.04`

	meteorDesc = `Meteor: build web and mobile apps in pure JavaScript

•  Meteor 1.0.1
•  Ubuntu 14.04`

	djangoDesc = `Django: the web framework for perfectionists with deadlines

•  Django 1.7.1
•  Python 3.4.0
•  IPython 2.3.1
•  Gunicorn 19.1.1
•  Ubuntu 14.04`

	vanillaDesc = `A minimalist development environment

This template contains Ubuntu 14.04 with useful developer tools preinstalled.`
)
