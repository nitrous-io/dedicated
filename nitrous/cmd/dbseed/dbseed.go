package main

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Sirupsen/logrus"
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
)

var (
	providerToSeed = "docker"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

type SeedFn func(db.QueryReceiver) error

var seeds = []SeedFn{users, regions, plans, providertemplates, templates, creditTypes}

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatal(envvars.DBEnvVar, "is not defined")
	}

	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatal(err)
	}
	if err := dbConn.MigrateAll(""); err != nil {
		logrus.Infof("Migrations failed: %v", err)
	}
	defer dbConn.DB.Close()

	tx, err := dbConn.Beginx()
	if err != nil {
		logrus.Fatal(err)
	}

	for _, f := range seeds {
		if err := f(tx); err != nil {
			tx.Rollback()
			logrus.Fatal(err)
		}
	}

	if err := tx.Commit(); err != nil {
		logrus.Fatal(err)
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
		logrus.Infof("Added user %+v", u)
	}
	return nil
}

func regions(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "regions")
	if err != nil || !ok {
		return err
	}
	regions := []region.Region{
		{Slug: "usw1", Rank: 100, Name: "US West"},
		{Slug: "use1", Rank: 200, Name: "US East"},
		{Slug: "euw1", Rank: 300, Name: "Europe"},
		{Slug: "apse1", Rank: 400, Name: "Asia"},
	}
	for _, r := range regions {
		if err := r.Insert(d); err != nil {
			return err
		}
		logrus.Infof("Added region %+v", r)
	}
	return nil
}

func plans(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "plans")
	if err != nil || !ok {
		return err
	}

	basic := &plan.Plan{Slug: "basic", Name: "Basic", StripePlanID: sql.NullString{"basic", true}, Type: plan.TypePrepaid, MaxContainers: 1, Price: 900, CPU: "1", Memory: "1 GB", Storage: "10 GB"}
	if err := basic.Insert(d); err != nil {
		return err
	}

	free := &plan.Plan{Slug: "free", Name: "Starter", StripePlanID: sql.NullString{"free", true}, Type: plan.TypePrepaid, MaxContainers: 1, Price: 0, CPU: "1", Memory: "1 GB", Storage: "5GB"}
	if err := free.Insert(d); err != nil {
		return err
	}

	plans := []*plan.Plan{
		{Slug: "standard", Name: "Standard", StripePlanID: sql.NullString{"standard", true}, Type: plan.TypePrepaid, MaxContainers: 3, Price: 2900, CPU: "1", Memory: "2 GB", Storage: "25 GB", MostPopular: true},
		{Slug: "plus", Name: "Plus", StripePlanID: sql.NullString{"plus", true}, Type: plan.TypePrepaid, MaxContainers: 10, Price: 7900, CPU: "2", Memory: "4 GB", Storage: "50 GB"},
		{Slug: "org-plus", Name: "Plus", StripePlanID: sql.NullString{"org-plus", true}, Type: plan.TypePrepaid, MaxContainers: 20, Price: 11900, CPU: "2", Memory: "4 GB", Storage: "80 GB", Org: true},
		{Slug: "org-pro", Name: "Pro", StripePlanID: sql.NullString{"org-pro", true}, Type: plan.TypePrepaid, MaxContainers: 50, Price: 24900, CPU: "2", Memory: "8 GB", Storage: "200 GB", Org: true},
	}
	for _, p := range plans {
		if err := p.Insert(d); err != nil {
			return err
		}
		logrus.Infof("Added plan %+v", p)
	}

	return nil
}

func providertemplates(d db.QueryReceiver) error {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE provider = $1", "provider_templates")
	if err := d.QueryRowx(query, providerToSeed).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		logrus.Infof("%d provider_templates found for provider %s, skipping provider_templates seeds", count, providerToSeed)
		return nil
	}

	plans := []plan.Plan{}
	if err := d.Select(&plans, "SELECT * FROM plans"); err != nil {
		return err
	}
	regions := []region.Region{}
	if err := d.Select(&regions, "SELECT * FROM regions"); err != nil {
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
			if err := pt.Insert(d); err != nil {
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
		railsJSON     = cm.templateJSON(defaultPorts, false)
		djangoJSON    = cm.templateJSON(defaultPorts, false)
		phpApacheJSON = cm.templateJSON(defaultPorts, false)
		phpNginxJSON  = cm.templateJSON(defaultPorts, false)
		meteorJSON    = cm.templateJSON(defaultPorts, false)
		wordpressJSON = cm.templateJSON(defaultPorts, false)
		discourseJSON = cm.templateJSON(defaultPorts, false)
		laravelJSON   = cm.templateJSON(defaultPorts, false)
		meanJSON      = cm.templateJSON(defaultPorts, false)
		aspnetJSON    = cm.templateJSON(defaultPorts, false)
		gcloudJSON    = cm.templateJSON(defaultPorts, false)

		html5JSON  = cm.templateJSON(defaultPorts, false)
		rubyJSON   = cm.templateJSON(defaultPorts, false)
		pythonJSON = cm.templateJSON(defaultPorts, false)
		nodejsJSON = cm.templateJSON(defaultPorts, false)
		iojsJSON   = cm.templateJSON(defaultPorts, false)
		golangJSON = cm.templateJSON(defaultPorts, false)
		ubuntuJSON = cm.templateJSON(defaultPorts, false)

		dindJSON = cm.templateJSON(defaultPorts, true)
	)

	if cm.err != nil {
		return err
	}

	templates := []template.Template{
		// stack templates
		newTemplate("Ruby on Rails", "rails", "quay.io/nitrous/rails:latest", railsJSON, template.TypeStack, railsDesc),
		newTemplate("Django", "django", "quay.io/nitrous/django:latest", djangoJSON, template.TypeStack, djangoDesc),
		newTemplate("PHP (LAMP)", "php-apache", "quay.io/nitrous/php-apache:latest", phpApacheJSON, template.TypeStack, phpApacheDesc),
		newTemplate("PHP (LEMP)", "php-nginx", "quay.io/nitrous/php-nginx:latest", phpNginxJSON, template.TypeStack, phpNginxDesc),
		newTemplate("Meteor", "meteor", "quay.io/nitrous/meteor:latest", meteorJSON, template.TypeStack, meteorDesc),
		newTemplate("Wordpress", "wordpress", "quay.io/nitrous/wordpress:latest", wordpressJSON, template.TypeStack, wordpressDesc),
		newTemplate("Discourse", "discourse", "quay.io/nitrous/discourse:latest", discourseJSON, template.TypeStack, discourseDesc),
		newTemplate("Laravel", "laravel", "quay.io/nitrous/laravel:latest", laravelJSON, template.TypeStack, laravelDesc),
		newTemplate("MEAN", "mean", "quay.io/nitrous/mean:latest", meanJSON, template.TypeStack, meanDesc),
		newTemplate("ASP.NET", "aspnet", "quay.io/nitrous/aspnet:latest", aspnetJSON, template.TypeStack, aspnetDesc),
		newTemplate("Google Cloud SDK", "gcloud", "quay.io/nitrous/gcloud:latest", gcloudJSON, template.TypeStack, gcloudDesc),
		// basic templates
		newTemplate("HTML5", "html5", "quay.io/nitrous/nginx:latest", html5JSON, template.TypeBasic, html5Desc),
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
		logrus.Infof("Added template %+v", t)
	}

	return nil
}

func creditTypes(d db.QueryReceiver) error {
	ok, err := shouldSeed(d, "credit_types")
	if err != nil || !ok {
		return err
	}

	// we keep Signup_Credit in db but we no longer want to apply it, so it's not Active
	cType := &credittype.CreditType{
		Name:         credittype.SignupCreditType,
		Description:  sql.NullString{"Awarded to users for signing up", true},
		Active:       false,
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
		logrus.Infof("%d %s found, skipping %s seeds", c, tableName, tableName)
		return false, nil
	case c == 0:
		return true, nil
	default: // if it's still -1
		logrus.Errorf("Unexpected count %v", c)
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
	goDesc        = `Go: an open source programming language that makes it easy to build simple, reliable, and efficient software.`
	phpApacheDesc = `PHP with Apache (LAMP): a popular general-purpose scripting language that is especially suited to web development.`
	phpNginxDesc  = `PHP with Nginx (LEMP): a popular general-purpose scripting language that is especially suited to web development.`
	html5Desc     = `HTML5: a static HTML5 website development environment with Nginx.`
	nodejsDesc    = `Node.js: a platform built on Chrome's JavaScript runtime for easily building fast, scalable network applications.`
	iojsDesc      = `io.js: a platform built on Chrome's JavaScript runtime for easily building fast, scalable network applications.`
	pythonDesc    = `Python: a programming language that lets you work quickly and integrate systems more effectively.`
	rubyDesc      = `Ruby: a dynamic, open source programming language with a focus on simplicity and productivity.`
	dockerDesc    = `A development environment with Docker set up.`
	railsDesc     = `Ruby on Rails: an open-source framework optimized for programmer happiness and sustainable productivity.`
	wordpressDesc = `Wordpress: a free and open-source blogging tool and a content management system.`
	discourseDesc = `Discourse: an open source Internet forum software application.`
	gcloudDesc    = `Google Cloud SDK: tools and libraries that enable you to easily create and manage resources on Google Cloud Platform.`
	laravelDesc   = `Laravel: a free, open source PHP web application framework.`
	meanDesc      = `MEAN: an opinionated full-stack JavaScript framework.`
	meteorDesc    = `Meteor: a complete platform for building web and mobile apps in pure JavaScript.`
	djangoDesc    = `Django: a Python web framework for perfectionists with deadlines.`
	aspnetDesc    = `ASP.NET: a free web framework for building great Web sites and Web applications.`
	vanillaDesc   = `Ubuntu: A minimalist development environment.`
)
