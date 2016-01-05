package pendingmembers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/pendingmembers")
}

var _ = Describe("controllers/pendingmembers", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil, nil)
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/pending_members/{code}", func() {
		var (
			o      *org.Org
			om     *orgmembership.OrgMembership
			params *url.Values
		)

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err := o.Insert(db)
			Expect(err).To(BeNil())

			om, err = orgmembership.AddMember(db, o.ID, "bob@pendingorg.com", "member")
			Expect(err).To(BeNil())

			u := &user.User{
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

			cnr := &container.Container{
				CID:    "cafebabe",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container",
				Name:   sql.NullString{"Java", true},
			}
			err = cnr.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, invite_email, invite_code) VALUES ($1, 'bob@pendingcollab.com', '22x2') RETURNING ID`, cnr.ID)
			Expect(err).To(BeNil())
		})

		doRequest := func(code string) {
			var err error
			url := fmt.Sprintf("/api/v0/pending_members/%s", code)
			req, err = test.NewRequest("GET", url, params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when member with a given invite code does not exist", func() {
			It("responds with 404 not found", func() {
				doRequest(om.InviteCode.String + "9000")
				Expect(res.Code).To(Equal(http.StatusNotFound))

				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("not_found"))
			})
		})

		Context("when member with a valid invite code exists in org memberships ", func() {
			It("responds OK and returns member data", func() {
				doRequest(om.InviteCode.String)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Member *serializers.OrgMember `json:"member"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Member.PendingEmail).To(Equal("bob@pendingorg.com"))
				Expect(j.Member.Role).To(Equal("member"))
				Expect(j.Member.UserID).To(BeZero())
				Expect(j.Member.Username).To(BeZero())
				Expect(j.Member.Email).To(BeZero())
			})
		})

		Context("when member with a valid invite code exists in container collabs", func() {
			It("responds OK and returns member data", func() {
				doRequest("22x2")
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Member *serializers.ContainerCollabUser `json:"member"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Member.PendingEmail).To(Equal("bob@pendingcollab.com"))
				Expect(j.Member.UserID).To(BeZero())
				Expect(j.Member.Username).To(BeZero())
			})
		})
	})
})
