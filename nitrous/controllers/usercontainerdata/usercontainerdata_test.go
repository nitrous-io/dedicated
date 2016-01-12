package usercontainerdata_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	ucd "github.com/nitrous-io/dedicated/nitrous/models/usercontainerdata"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/usercontainerdata")
}

var _ = Describe("controllers/usercontainerdata", func() {
	var (
		d       *db.Database
		svr     *server.Server
		handler *mux.Router
		res     *httptest.ResponseRecorder

		u  *user.User
		t  *token.Token
		hs *test.HostStack
		c  *container.Container
	)

	insert := func(d db.QueryReceiver, uc *ucd.UserContainerData) error {
		nstmt, err := d.PrepareNamed(`INSERT INTO user_container_data
		(user_id, container_id, metadata)
		VALUES (:user_id, :container_id, :metadata) RETURNING *;`)
		if err != nil {
			return err
		}
		return nstmt.QueryRowx(uc).StructScan(uc)
	}

	BeforeEach(func() {
		var err error

		d, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = test.NewTestServer("", d, nil)
		handler = svr.Router()
		res = httptest.NewRecorder()

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(d, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(d)
		Expect(err).To(BeNil())

		hs, err = test.CreateHostStack(d, &test.HostStack{
			H: &host.Host{
				Slug:      "foo",
				IPAddress: sql.NullString{"127.0.0.1", true},
			},
			U: u,
		})
		Expect(err).To(BeNil())

		c = &container.Container{
			CID:    "deadeef",
			UserID: u.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "my-1st-container",
		}
		Expect(c.Insert(d)).To(BeNil())
	})

	AfterEach(func() {
		if d != nil {
			d.Close()
		}
	})

	Describe("PUT /containers/:slug/user_data", func() {
		BeforeEach(func() {
		})

		doRequest := func(slug, key, value string) {
			params := &url.Values{
				"key":   []string{key},
				"value": []string{value},
			}
			req, err := test.NewAuthedFormRequest(t, "PUT", fmt.Sprintf("/api/v0/containers/%s/user_data", slug), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return an error if the `key` is not provided", func() {
			doRequest(c.Slug, "", "value")
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("`key` is required"))

			_, err := ucd.Find(d, u.ID, c.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("should return an error if the `value` is not provided", func() {
			doRequest(c.Slug, "key", "")
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("`value` is required"))

			_, err := ucd.Find(d, u.ID, c.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("should insert new metadata if the record does not exist", func() {
			doRequest(c.Slug, "new", "value")
			Expect(res.Code).To(Equal(http.StatusOK))

			uc, err := ucd.Find(d, u.ID, c.ID)
			Expect(err).To(BeNil())
			Expect(uc.Metadata.Map["new"].String).To(Equal("value"))
		})

		It("should update metadata if the record does exist", func() {
			record := &ucd.UserContainerData{
				UserID:      u.ID,
				ContainerID: c.ID,
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"some": sql.NullString{"value", true},
					},
				},
			}
			Expect(insert(d, record)).To(BeNil())

			doRequest(c.Slug, "new", "value")
			Expect(res.Code).To(Equal(http.StatusOK))

			uc, err := ucd.Find(d, u.ID, c.ID)
			Expect(err).To(BeNil())
			Expect(uc.Metadata.Map["some"].String).To(Equal("value"))
			Expect(uc.Metadata.Map["new"].String).To(Equal("value"))
		})
	})
})
