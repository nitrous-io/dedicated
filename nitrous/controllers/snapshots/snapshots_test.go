package snapshots_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/snapshots")
}

var _ = Describe("controllers/snapshots", func() {
	var (
		db  *ndb.Database
		svr *server.Server

		u      *user.User
		o1, o2 *org.Org
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil, nil)

		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

		t := &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		Expect(t.Insert(db)).To(BeNil())

		o1 = &org.Org{
			Name:  "ze robot labs",
			Email: "admin@zerobotlabs.com",
			Slug:  "zerobotlabs",
		}
		Expect(o1.Insert(db)).To(BeNil())

		o2 = &org.Org{
			Name:  "Path",
			Email: "admin@path.com",
			Slug:  "path",
		}
		Expect(o2.Insert(db)).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/orgs/{org_slug}/snapshots", func() {
		doRequest := func(orgSlug string) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/orgs/%s/snapshots", orgSlug)
			req, err := test.NewRequest("GET", url, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("responds with HTTP 404 Not Found when org does not exist", func() {
			res := doRequest("non-existent-org")
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("not_found"))
			Expect(errDesc).To(Equal("org does not exist"))
		})

		It("responds with HTTP 403 Forbidden when user does not belong in the org", func() {
			res := doRequest(o1.Slug)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you are not a member of this org"))
		})

		Context("when user is a member of the org", func() {
			BeforeEach(func() {
				// Add user to both orgs.
				err := orgmembership.AddMember(db, o1.ID, u.ID, "admin")
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o2.ID, u.ID, "admin")
				Expect(err).To(BeNil())

				// Add snapshots to one of the orgs.
				s := &snapshot.Snapshot{
					UserID:        u.ID,
					OrgID:         sql.NullInt64{o1.ID, true},
					Namespace:     "zerobotlabs",
					Name:          "nestorbot",
					Tag:           "latest",
					Repository:    "zerobotlabs/nestorbot",
					DisplayName:   "Nestorbot dev",
					OriginalImage: "golang:latest",
				}
				Expect(s.Insert(db)).To(BeNil())
			})

			It("responds with no snapshots if org does not have any snapshots", func() {
				res := doRequest(o2.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Snapshots []*serializers.Snapshot `json:"snapshots"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Snapshots).To(HaveLen(0))
			})

			It("responds with snapshots belonging to the org", func() {
				res := doRequest(o1.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Snapshots []*serializers.Snapshot `json:"snapshots"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Snapshots).To(HaveLen(1))

				s := j.Snapshots[0]
				Expect(s.UserID).To(Equal(u.ID))
				Expect(o1.ID).To(Equal(o1.ID))
				Expect(s.DisplayName).To(Equal("Nestorbot dev"))
			})
		})
	})
})
