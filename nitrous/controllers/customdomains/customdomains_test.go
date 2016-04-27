package customdomains_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/customdomain"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/customdomains")
}

var _ = Describe("controllers/customdomains", func() {
	var (
		db  *db.Database
		u   *user.User
		t   *token.Token
		cnr *container.Container

		svr *server.Server
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		svr = test.NewTestServer("", db, test.NewFakeTracker())

		u = &user.User{
			Username: "owner",
			Email:    "owner@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		hs, err := test.CreateHostStack(db, &test.HostStack{
			U: u,
		})
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		cnr = &container.Container{
			CID:    "cafebabe",
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container",
			Name:   sql.NullString{"Java", true},
		}
		err = cnr.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /containers/{slug}/custom_domains", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/containers/%s/custom_domains", cnr.Slug), params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("creates a custom domain for the container", func() {
			params := &url.Values{}
			params.Set("slug", cnr.Slug)
			params.Set("domain", "www.gomama.com")
			params.Set("port", "80")
			params.Set("container_port", "4000")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusCreated))

			cd := &customdomain.CustomDomain{}
			err := db.Get(cd, "SELECT * FROM custom_domains LIMIT 1")
			Expect(err).To(BeNil())
			Expect(cd).ToNot(BeNil())

			Expect(cd.Domain).To(Equal("www.gomama.com"))
			Expect(cd.Port).To(Equal(int64(80)))
			Expect(cd.ContainerPort).To(Equal(int64(4000)))
		})

		It("strips domain name protocol", func() {
			params := &url.Values{}
			params.Set("slug", cnr.Slug)
			params.Set("port", "80")
			params.Set("container_port", "9000")

			params.Set("domain", "http://www.gomama.com")
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusCreated))

			cd := &customdomain.CustomDomain{}
			err := db.Get(cd, "SELECT * FROM custom_domains WHERE domain = $1", "www.gomama.com")
			Expect(err).To(BeNil())
			Expect(cd).ToNot(BeNil())

			params.Set("domain", "https://www.gobaba.com")
			res = doRequest(params)
			Expect(res.Code).To(Equal(http.StatusCreated))

			cd = &customdomain.CustomDomain{}
			err = db.Get(cd, "SELECT * FROM custom_domains WHERE domain = $1", "www.gobaba.com")
			Expect(err).To(BeNil())
			Expect(cd).ToNot(BeNil())
		})

		It("returns error if domain name is invalid", func() {
			params := &url.Values{}
			params.Set("slug", cnr.Slug)
			params.Set("port", "80")
			params.Set("container_port", "9000")

			params.Set("domain", "www_.gomama._com")
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			params.Set("domain", "http://www.gomama.c.c..c.c.com")
			res = doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})

		It("returns error if port is invalid", func() {
			params := &url.Values{}
			params.Set("slug", cnr.Slug)
			params.Set("domain", "www.gomama.com")
			params.Set("port", "67000")
			params.Set("container_port", "9000")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})

		It("returns error if container port is invalid", func() {
			params := &url.Values{}
			params.Set("slug", cnr.Slug)
			params.Set("domain", "www.gomama.com")
			params.Set("port", "80")
			params.Set("container_port", "10000")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})

		It("returns error if domain name is duplicated", func() {
			params := &url.Values{}
			params.Set("slug", cnr.Slug)
			params.Set("domain", "www.gomama.com")
			params.Set("port", "80")
			params.Set("container_port", "4000")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusCreated))

			res = doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})
	})
})
