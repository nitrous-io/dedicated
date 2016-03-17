package template_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/template")
}

var _ = Describe("models/template", func() {
	var (
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("All()", func() {
		BeforeEach(func() {
			var err error
			config := `{
				"ports": ["private:5432:5432"],
				"volumes": ["database:/var/lib/postgresql/data"],
				"env": ["ABC=true", "123=false"],
				"privileged": true,
				"publish_all": false
			}`
			_, err = db.Exec(`INSERT INTO templates (slug, name, description, config, image) VALUES ($1, $2, $3, $4, $5);`, "rails-stack", "Ruby on Rails", "RoR template", config, "ubuntu-bare")
			Expect(err).To(BeNil())

			config = `{
				"ports": ["public:8000:8000"],
				"volumes": ["swap:/swp"],
				"env": ["123=true", "INSTALLED=ok"],
				"privileged": false,
				"publish_all": true
			}`
			_, err = db.Exec(`INSERT INTO templates (slug, name, description, config, image) VALUES ($1, $2, $3, $4, $5);`, "php", "PHP", "PHP template", config, "freebsd:10.1")
			Expect(err).To(BeNil())
		})

		It("returns all templates", func() {
			tmpls, err := template.All(db)
			Expect(err).To(BeNil())
			Expect(tmpls).To(HaveLen(2))
			Expect([]string{tmpls[0].Slug, tmpls[1].Slug}).To(ConsistOf([]string{"rails-stack", "php"}))
		})
	})

	Describe("ParsedConfig()", func() {
		It("returns pointer to a TemplateConfig unmarshalled from .Config", func() {
			config := `{
				"ports": ["private:5432:5432", "public:2222:22"],
				"volumes": ["database:/var/lib/postgresql/data", "swap:/var/lib/swap"],
				"env": ["ABC=true", "123=false"],
				"privileged": true,
				"publish_all": false
			}`
			_, err := db.Exec(`INSERT INTO templates (slug, name, description, config, image) VALUES ($1, $2, $3, $4, $5);`, "rails-stack", "Ruby on Rails", "RoR template", config, "freebsd:10.1")
			Expect(err).To(BeNil())

			tmpl, err := template.FindBySlug(db, "rails-stack")
			Expect(err).To(BeNil())

			cfg, err := tmpl.ParsedConfig()
			Expect(err).To(BeNil())
			Expect(cfg).To(Equal(&template.TemplateConfig{
				Privileged: true,
				PublishAll: false,
				Env:        []string{"ABC=true", "123=false"},
				Ports:      []string{"private:5432:5432", "public:2222:22"},
				Volumes:    []string{"database:/var/lib/postgresql/data", "swap:/var/lib/swap"},
			}))
		})
	})

	Describe("Insert()", func() {
		It("persists the template in the database", func() {
			tmpl := &template.Template{
				Slug:        "rails-stack",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails stack template", true},
				Image:       "ubuntu:18.04",
				Config:      `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true }`,
				Type:        template.TypeStack,
			}
			err := tmpl.Insert(db)
			Expect(err).To(BeNil())

			tmpl2, err := template.FindBySlug(db, "rails-stack")
			Expect(err).To(BeNil())

			Expect(tmpl2.Slug).To(Equal("rails-stack"))
			Expect(tmpl2.Name).To(Equal(sql.NullString{"Ruby on Rails", true}))
			Expect(tmpl2.Description).To(Equal(sql.NullString{"A Rails stack template", true}))
			Expect(tmpl2.Image).To(Equal("ubuntu:18.04"))
			Expect(tmpl2.Config).To(Equal(`{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true }`))
			Expect(tmpl2.Type).To(Equal(template.TypeStack))
			Expect(tmpl2.CreatedAt).NotTo(BeNil())
			Expect(tmpl2.UpdatedAt).NotTo(BeNil())
		})

		It("returns ErrConfigInvalid when .Config is invalid JSON", func() {
			tmpl := &template.Template{
				Slug:        "rails-stack",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails stack template", true},
				Config:      `{ invalid: "JSON" }`,
			}
			err := tmpl.Insert(db)
			Expect(err).To(Equal(template.ErrConfigInvalid))
		})

		It("returns ErrInvalidType when invalid type is provided", func() {
			tmpl := &template.Template{
				Slug:        "rails-stack",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails stack template", true},
				Config:      `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true }`,
				Type:        "invalid-type",
			}
			err := tmpl.Insert(db)
			Expect(err).To(Equal(template.ErrInvalidType))
		})
	})

	Describe("Update()", func() {
		BeforeEach(func() {
			tmpl := &template.Template{
				Slug:        "rails-stack",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails stack template", true},
				Image:       "ubuntu:18.04",
				Config:      `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true }`,
				Type:        template.TypeStack,
			}
			err := tmpl.Insert(db)
			Expect(err).To(BeNil())
		})
		It("updates the template in the database", func() {
			tmpl, err := template.FindBySlug(db, "rails-stack")
			Expect(err).To(BeNil())

			tmpl.Description = sql.NullString{"A Rails stack template with Ubuntu 18.10", true}
			tmpl.Image = "ubuntu:18.10"
			tmpl.Config = `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`

			err = tmpl.Update(db)
			Expect(err).To(BeNil())

			Expect(tmpl.Slug).To(Equal("rails-stack"))
			Expect(tmpl.Name).To(Equal(sql.NullString{"Ruby on Rails", true}))
			Expect(tmpl.Description).To(Equal(sql.NullString{"A Rails stack template with Ubuntu 18.10", true}))
			Expect(tmpl.Image).To(Equal("ubuntu:18.10"))
			Expect(tmpl.Config).To(Equal(`{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`))
			Expect(tmpl.Type).To(Equal(template.TypeStack))
			Expect(tmpl.CreatedAt).NotTo(BeNil())
			Expect(tmpl.UpdatedAt).NotTo(BeNil())
		})

		It("returns ErrConfigInvalid when .Config is invalid JSON", func() {
			tmpl, err := template.FindBySlug(db, "rails-stack")
			Expect(err).To(BeNil())

			tmpl.Config = `{ invalid: "JSON" }`

			err = tmpl.Update(db)
			Expect(err).To(Equal(template.ErrConfigInvalid))
		})

		It("returns ErrInvalidType when invalid type is provided", func() {
			tmpl, err := template.FindBySlug(db, "rails-stack")
			Expect(err).To(BeNil())

			tmpl.Type = "invalid-type"

			err = tmpl.Update(db)
			Expect(err).To(Equal(template.ErrInvalidType))
		})
	})
})
