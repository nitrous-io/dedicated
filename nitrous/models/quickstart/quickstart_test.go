package quickstart_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/quickstart"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/quickstart")
}

var _ = Describe("models/quickstart", func() {
	var (
		db   *db.Database
		hs   *test.HostStack
		c    *container.Container
		tmpl *template.Template
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:          "foo",
				IPAddress:     sql.NullString{"127.0.0.1", true},
				MaxContainers: sql.NullInt64{1, true},
			},
		})
		Expect(err).To(BeNil())

		c = &container.Container{
			CID:    "container-id-id-id",
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "rails-test-container",
		}
		Expect(c.Insert(db)).To(BeNil())

		tmpl = &template.Template{
			Slug:        "rails",
			Name:        sql.NullString{"Ruby on Rails", true},
			Description: sql.NullString{"A ruby on rails stack template", true},
			Image:       "rails:latest",
			Config: `{
						"privileged": true,
						"publish_all": false,
						"ports": ["public::2341", "public::8931"],
						"env": ["SHELL=powershell"]
					}`,
			Type: template.TypeStack,
		}
		Expect(tmpl.Insert(db)).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Insert()", func() {
		It("persists the quickstart in the database", func() {
			q1 := &quickstart.Quickstart{
				UserID:      hs.U.ID,
				ContainerID: c.ID,
				TemplateID:  tmpl.ID,
				RepoURL:     "https://github.com/yeouchien/discourse",
			}
			Expect(q1.Insert(db)).To(BeNil())

			q2 := &quickstart.Quickstart{}
			Expect(db.Get(q2, `SELECT * FROM quickstarts LIMIT 1`)).To(BeNil())
			Expect(q2).NotTo(BeNil())
			Expect(q2.UserID).To(Equal(hs.U.ID))
			Expect(q2.ContainerID).To(Equal(c.ID))
			Expect(q2.TemplateID).To(Equal(tmpl.ID))
			Expect(q2.RepoURL).To(Equal("https://github.com/yeouchien/discourse"))
			Expect(q2.CreatedAt).NotTo(BeNil())
			Expect(q2.UpdatedAt).NotTo(BeNil())
		})

		It("returns error when some error occurs", func() {
			q := &quickstart.Quickstart{
				UserID:      100000,
				ContainerID: 2123981,
				TemplateID:  31231312,
				RepoURL:     "https://github.com/yeouchien/discourse",
			}
			Expect(q.Insert(db)).NotTo(BeNil())
		})
	})
})
