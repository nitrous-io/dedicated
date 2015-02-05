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
	logr           *logger.Logger
	providerToSeed string
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

type SeedFn func(db.QueryReceiver) error

var seeds = []SeedFn{users, regions, plans, providertemplates, templates, creditTypes}

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}
	providerToSeed = envvars.DefaultProvider
	if providerToSeed != "digitalocean" && providerToSeed != "docker" {
		logr.Fatalf("DEFAULT_PROVIDER %s must be one of digitalocean or docker", providerToSeed)
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
		{Slug: "sfo", Rank: 100, Name: "US West"},
		{Slug: "nyc", Rank: 200, Name: "US East"},
		{Slug: "ams", Rank: 300, Name: "Europe"},
		{Slug: "sgp", Rank: 400, Name: "Asia"},
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
		{Slug: "2gb", Name: "Turbo", MaxContainers: 5, Price: 4500, CPU: "2", Memory: "2 GB", Storage: "30 GB", MostPopular: true},
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
			pt := &providertemplate.ProviderTemplate{
				Name:     fmt.Sprintf("%s-%s-%s", providerToSeed, p.Slug, r.Slug),
				Provider: providerToSeed,
				RegionID: r.ID,
				PlanID:   p.ID,
				Config: hstore.Hstore{
					Map: map[string]sql.NullString{
						"region_id": {r.Slug, true},
						"size_id":   {p.Slug, true},
					},
				},
			}
			if providerToSeed == "digitalocean" {
				pt.Config.Map["image_id"] = sql.NullString{DigitalOceanImageID, true}
			}
			err := pt.Insert(d)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type ConfigMarshaller struct {
	err error
}

func (cm *ConfigMarshaller) templateJSON(ports []string, privileged bool) []byte {
	if cm.err != nil {
		return nil
	}

	cfg := template.TemplateConfig{
		Privileged: privileged,
		PublishAll: true,
		Env:        nil,
		Ports:      ports,
		Volumes:    nil,
	}
	var bs []byte
	bs, cm.err = json.Marshal(cfg)
	return bs
}

func templates(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "templates")
	if err != nil || !ok {
		return err
	}

	var (
		defaultPorts = []string{}
	)

	cm := &ConfigMarshaller{}

	var (
		railsJSON   = cm.templateJSON(defaultPorts, false)
		djangoJSON  = cm.templateJSON(defaultPorts, false)
		phpLampJson = cm.templateJSON(defaultPorts, false)
		meteorJSON  = cm.templateJSON(defaultPorts, false)
		laravelJSON = cm.templateJSON(defaultPorts, false)
		meanJSON    = cm.templateJSON(defaultPorts, false)
		aspnetJSON  = cm.templateJSON(defaultPorts, false)

		rubyJSON   = cm.templateJSON(defaultPorts, false)
		pythonJSON = cm.templateJSON(defaultPorts, false)
		nodejsJSON = cm.templateJSON(defaultPorts, false)
		iojsJSON   = cm.templateJSON(defaultPorts, false)
		golangJSON = cm.templateJSON(defaultPorts, false)
		ubuntuJSON = cm.templateJSON(defaultPorts, false)
		dindJSON   = cm.templateJSON(defaultPorts, true)
	)

	if cm.err != nil {
		return err
	}

	templates := []template.Template{
		// stack templates
		newTemplate("Ruby on Rails", "rails", "quay.io/nitrous/rails:latest", railsJSON, template.TypeStack, railsDesc),
		newTemplate("Django", "django", "quay.io/nitrous/django:latest", djangoJSON, template.TypeStack, djangoDesc),
		newTemplate("PHP (LAMP)", "php", "quay.io/nitrous/php:latest", phpLampJson, template.TypeStack, phpLampDesc),
		newTemplate("Meteor", "meteor", "quay.io/nitrous/meteor:latest", meteorJSON, template.TypeStack, meteorDesc),
		newTemplate("Laravel", "laravel", "quay.io/nitrous/laravel:latest", laravelJSON, template.TypeStack, laravelDesc),
		newTemplate("MEAN", "mean", "quay.io/nitrous/mean:latest", meanJSON, template.TypeStack, meanDesc),
		newTemplate("ASP.NET", "aspnet", "quay.io/nitrous/aspnet:latest", aspnetJSON, template.TypeStack, aspnetDesc),
		// basic templates
		newTemplate("Ruby", "ruby", "quay.io/nitrous/ruby:latest", rubyJSON, template.TypeBasic, rubyDesc),
		newTemplate("Python", "python", "quay.io/nitrous/python:latest", pythonJSON, template.TypeBasic, pythonDesc),
		newTemplate("Node.js", "nodejs", "quay.io/nitrous/nodejs:latest", nodejsJSON, template.TypeBasic, nodejsDesc),
		newTemplate("io.js", "iojs", "quay.io/nitrous/iojs:latest", iojsJSON, template.TypeBasic, iojsDesc),
		newTemplate("Go", "golang", "quay.io/nitrous/golang:latest", golangJSON, template.TypeBasic, goDesc),
		newTemplate("Ubuntu", "ubuntu", "quay.io/nitrous/ubuntu:latest", ubuntuJSON, template.TypeBasic, vanillaDesc),
		// advanced template
		newTemplate("Docker", "docker", "quay.io/nitrous/docker:latest", dindJSON, template.TypeAdvanced, dockerDesc),
	}
	for _, t := range templates {
		if err := t.Insert(d); err != nil {
			return err
		}
		logr.Infof("Added template %+v", t)
	}

	return nil
}

func creditTypes(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "credit_types")
	if err != nil || !ok {
		return err
	}

	cType := &credittype.CreditType{
		Name:         credittype.SignupCreditType,
		Description:  sql.NullString{"Awarded to users for signing up", true},
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

func newTemplate(name, slug, image string, cfgJSON []byte, tempType, desc string) template.Template {
	return template.Template{
		Name:        sql.NullString{name, true},
		Config:      string(cfgJSON),
		Slug:        slug,
		Image:       image,
		Type:        tempType,
		Description: sql.NullString{desc, true},
	}
}

// Template descriptions
const (
	goDesc = `Go: an open source programming language that makes it easy to build simple, reliable, and efficient software

•  Go 1.4
•  Ubuntu 14.04`

	phpLampDesc = `PHP: a popular general-purpose scripting language that is especially suited to web development

•  PHP 5.5
•  Composer 1.0
•  MySQL 5.5
•  Apache 2.4
•  Ubuntu 14.04`

	nodejsDesc = `Node.js: a platform built on Chrome's JavaScript runtime for easily building fast, scalable network applications

•  Node.js 0.10
•  NPM 2.3
•  NVM 0.23
•  Ubuntu 14.04`

	iojsDesc = `io.js: a platform built on Chrome's JavaScript runtime for easily building fast, scalable network applications

•  io.js 1.1
•  NPM 2.3
•  NVM 0.23
•  Ubuntu 14.04`

	pythonDesc = `Python: a programming language that lets you work quickly and integrate systems more effectively

•  Virtualenv 1.11
•  Python 3.4
•  Python 2.7
•  Ubuntu 14.04`

	rubyDesc = `Ruby: a dynamic, open source programming language with a focus on simplicity and productivity.

•  Ruby 2.2.0, 2.1.5 (default), 2.0.0, 1.9.3
•  Bundler 1.7.12
•  Ubuntu 14.04`

	dockerDesc = `A development environment with Docker set up.

•  Docker 1.4
•  Ubuntu 14.04`

	railsDesc = `Ruby on Rails: an open-source framework optimized for programmer happiness and sustainable productivity

•  Rails 4.2
•  Ruby 2.1.5
•  PostgreSQL 9.3
•  Ubuntu 14.04`

	laravelDesc = `Laravel, the PHP framework for web artisans

•  Laravel 4.2
•  PHP 5.5
•  PostgreSQL 9.3
•  Ubuntu 14.04`

	meanDesc = `MEAN: an opinionated full-stack JavaScript framework

•  MEAN 0.4
•  Node.js 0.10
•  MongoDB 2.6
•  Ubuntu 14.04`

	meteorDesc = `Meteor: build web and mobile apps in pure JavaScript

•  Meteor 1.0.1
•  Ubuntu 14.04`

	djangoDesc = `Django: the web framework for perfectionists with deadlines

•  Django 1.7
•  Python 3.4
•  IPython 2.3
•  Gunicorn 19.1
•  Ubuntu 14.04`

	aspnetDesc = `ASP.NET: a free web framework for building great Web sites and Web applications

•  ASP.NET 5
•  Mono 3.12
•  KRE 1.0
•  Ubuntu 14.04`

	vanillaDesc = `A minimalist development environment

This template contains Ubuntu 14.04 with useful developer tools preinstalled.`
)
