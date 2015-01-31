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

func (cm *ConfigMarshaller) templateJSON(repo string, ports []string, privileged bool) []byte {
	if cm.err != nil {
		return nil
	}
	c := template.Container{
		Privileged: privileged,
		PublishAll: false,
		Env:        nil,
		Image:      repo,
		Ports:      ports,
		Volumes:    nil,
	}

	cfg := template.TemplateConfig{
		Containers: []template.Container{c},
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
		stackPorts = []string{"public::22", "public::3000"}
		basicPorts = []string{"public::22"}
	)

	cm := &ConfigMarshaller{}

	var (
		railsJSON   = cm.templateJSON("quay.io/nitrous/rails:latest", stackPorts, false)
		laravelJSON = cm.templateJSON("quay.io/nitrous/laravel:latest", stackPorts, false)
		meanJSON    = cm.templateJSON("quay.io/nitrous/mean:latest", stackPorts, false)
		meteorJSON  = cm.templateJSON("quay.io/nitrous/meteor:latest", stackPorts, false)
		djangoJSON  = cm.templateJSON("quay.io/nitrous/django:latest", stackPorts, false)
		aspnetJSON  = cm.templateJSON("quay.io/nitrous/aspnet:latest", stackPorts, false)

		golangJSON = cm.templateJSON("quay.io/nitrous/golang:latest", basicPorts, false)
		nodejsJSON = cm.templateJSON("quay.io/nitrous/nodejs:latest", basicPorts, false)
		phpJSON    = cm.templateJSON("quay.io/nitrous/php:latest", basicPorts, false)
		pythonJSON = cm.templateJSON("quay.io/nitrous/python:latest", basicPorts, false)
		rubyJSON   = cm.templateJSON("quay.io/nitrous/ruby:latest", basicPorts, false)
		dindJSON   = cm.templateJSON("quay.io/nitrous/docker:latest", basicPorts, true)
		ubuntuJSON = cm.templateJSON("quay.io/nitrous/ubuntu:latest", basicPorts, false)
	)

	if cm.err != nil {
		return err
	}

	templates := []template.Template{
		// stack templates
		newTemplate("Ruby on Rails", "rails", railsJSON, template.TypeStack, railsDesc),
		newTemplate("Laravel", "laravel", laravelJSON, template.TypeStack, laravelDesc),
		newTemplate("MEAN", "mean", meanJSON, template.TypeStack, meanDesc),
		newTemplate("Meteor", "meteor", meteorJSON, template.TypeStack, meteorDesc),
		newTemplate("Django", "django", djangoJSON, template.TypeStack, djangoDesc),
		newTemplate("ASP.NET", "aspnet", aspnetJSON, template.TypeStack, aspnetDesc),
		// basic templates
		newTemplate("Go", "golang", golangJSON, template.TypeBasic, goDesc),
		newTemplate("Node.js", "nodejs", nodejsJSON, template.TypeBasic, nodejsDesc),
		newTemplate("PHP", "php", phpJSON, template.TypeBasic, phpDesc),
		newTemplate("Python", "python", pythonJSON, template.TypeBasic, pythonDesc),
		newTemplate("Ruby", "ruby", rubyJSON, template.TypeBasic, rubyDesc),
		newTemplate("Ubuntu", "ubuntu", ubuntuJSON, template.TypeBasic, vanillaDesc),
		// advanced template
		newTemplate("Docker", "docker", dindJSON, template.TypeAdvanced, dockerDesc),
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

	pythonDesc = `Python: a programming language that lets you work quickly and integrate systems more effectively

•  Virtualenv 1.11
•  Python 3.4
•  Python 2.7
•  Ubuntu 14.04`

	rubyDesc = `Ruby: a dynamic, open source programming language with a focus on simplicity and productivity.

•  Ruby 2.2
•  Bundler 1.7.12
•  Ubuntu 14.04`

	dockerDesc = `A development environment with Docker set up.

•  Docker 1.4
•  Ubuntu 14.04`

	railsDesc = `Ruby on Rails: an open-source framework optimized for programmer happiness and sustainable productivity

•  Rails 4.2
•  Ruby 2.2
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
