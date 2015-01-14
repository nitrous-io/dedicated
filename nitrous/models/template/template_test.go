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
				"containers": [
					{ "name": "Rails", "slug": "rails", "image": "nitrousio/rails:4.2", "ports": ["public:3000:3000", "public:2222:22"] },
					{ "name": "Postgres", "slug": "postgres", "image": "postgres:9.3", "ports": ["private:5432:5432"], "volumes": ["database:/var/lib/postgresql/data"]}
				],
				"volumes": [ { "name": "database", "slug": "database" } ]
			}`
			_, err = db.Exec(`INSERT INTO templates (slug, name, description, config) VALUES ($1, $2, $3, $4);`, "rails-starter", "Ruby on Rails", "RoR template", config)
			Expect(err).To(BeNil())

			config = `{
				"containers": [
					{ "name": "PHP", "slug": "php", "image": "nitrousio/php:latest", "ports": ["public:8000:8000"]" }
				]
			}`
			_, err = db.Exec(`INSERT INTO templates (slug, name, description, config) VALUES ($1, $2, $3, $4);`, "php", "PHP", "PHP template", config)
			Expect(err).To(BeNil())
		})

		It("returns all templates", func() {
			tmpls, err := template.All(db)
			Expect(err).To(BeNil())
			Expect(tmpls).To(HaveLen(2))
			Expect([]string{tmpls[0].Slug, tmpls[1].Slug}).To(ConsistOf([]string{"rails-starter", "php"}))
		})
	})

	Describe("ParsedConfig()", func() {
		It("returns pointer to a TemplateConfig unmarshalled from .Config", func() {
			config := `{
				"containers": [
					{ "name": "Rails", "slug": "rails", "image": "nitrousio/rails:4.2", "ports": ["public:3000:3000", "public:2222:22"], "cmd": "rails server" },
					{ "name": "Postgres", "slug": "postgres", "image": "postgres:9.3", "ports": ["private:5432:5432"], "volumes": ["database:/var/lib/postgresql/data"]}
				],
				"volumes": [ { "name": "database", "slug": "database" } ]
			}`
			_, err := db.Exec(`INSERT INTO templates (slug, name, description, config) VALUES ($1, $2, $3, $4);`, "rails-starter", "Ruby on Rails", "RoR template", config)
			Expect(err).To(BeNil())

			tmpl, err := template.Find(db, "rails-starter")
			Expect(err).To(BeNil())

			cfg, err := tmpl.ParsedConfig()
			Expect(err).To(BeNil())
			Expect(cfg).To(Equal(&template.TemplateConfig{
				Containers: []template.Container{
					{
						Name:       "Rails",
						Slug:       "rails",
						Image:      "nitrousio/rails:4.2",
						Cmd:        "rails server",
						Privileged: false,
						PublishAll: false,
						Env:        nil,
						Ports:      []string{"public:3000:3000", "public:2222:22"},
						Volumes:    nil,
					},
					{
						Name:       "Postgres",
						Slug:       "postgres",
						Image:      "postgres:9.3",
						Cmd:        "",
						Privileged: false,
						PublishAll: false,
						Env:        nil,
						Ports:      []string{"private:5432:5432"},
						Volumes:    []string{"database:/var/lib/postgresql/data"},
					},
				},
				Volumes: []template.Volume{{Slug: "database", Name: "database"}},
			}))
		})
	})

	Describe("Insert()", func() {
		It("persists the template in the database", func() {
			tmpl := &template.Template{
				Slug:        "rails-starter",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails starter template", true},
				Config:      `{ "containers": [ { "name": "Rails", "slug": "rails", "image": "rails:4.2" } ] }`,
				Type:        template.StarterType,
			}
			err := tmpl.Insert(db)
			Expect(err).To(BeNil())

			tmpl2, err := template.Find(db, "rails-starter")
			Expect(err).To(BeNil())

			Expect(tmpl2.Slug).To(Equal("rails-starter"))
			Expect(tmpl2.Name).To(Equal(sql.NullString{"Ruby on Rails", true}))
			Expect(tmpl2.Description).To(Equal(sql.NullString{"A Rails starter template", true}))
			Expect(tmpl2.Config).To(Equal(`{ "containers": [ { "name": "Rails", "slug": "rails", "image": "rails:4.2" } ] }`))
			Expect(tmpl2.Type).To(Equal(template.StarterType))
			Expect(tmpl2.CreatedAt).NotTo(BeNil())
			Expect(tmpl2.UpdatedAt).NotTo(BeNil())
		})

		It("returns ErrConfigInvalid when .Config is invalid JSON", func() {
			tmpl := &template.Template{
				Slug:        "rails-starter",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails starter template", true},
				Config:      `{ invalid: "JSON" }`,
			}
			err := tmpl.Insert(db)
			Expect(err).To(Equal(template.ErrConfigInvalid))
		})
	})
})
