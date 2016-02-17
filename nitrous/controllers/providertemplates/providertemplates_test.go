package providertemplates_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/providertemplates")
}

var _ = Describe("controllers/", func() {
	var (
		db      *db.Database
		svr     *server.Server
		handler *mux.Router

		authTokenInputs AuthTokenInputs

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = NewTestServer("", db, nil)
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

		handler = svr.Router()

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Name:     "foo",
			Email:    "foo@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/admin/provider_templates", func() {
		doRequest := func(url string) *httptest.ResponseRecorder {
			req, err := NewRequest("GET", url, nil)
			Expect(err).To(BeNil())

			req.Header.Set("Authorization", "Bearer "+t.Token)

			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			return res
		}

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest("/api/v0/admin/provider_templates")
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			var insertedPt1, insertedPt2 *providertemplate.ProviderTemplate

			BeforeEach(func() {
				db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
			})

			BeforeEach(func() {
				insertedPt1 = &providertemplate.ProviderTemplate{
					Name:     "do-512MB",
					Provider: "digitalocean",
					Config: hstore.Hstore{
						map[string]sql.NullString{
							"foo": sql.NullString{"bar", true},
						},
					},
				}

				hostStack := &HostStack{
					PT: insertedPt1,
				}

				_, err := CreateHostStack(db, hostStack)
				Expect(err).To(BeNil())

				insertedPt2 = &providertemplate.ProviderTemplate{
					Name:     "do-1GB",
					RegionID: insertedPt1.RegionID,
					PlanID:   insertedPt1.PlanID,
					Provider: "ec2",
				}

				err = insertedPt2.Insert(db)
				Expect(err).To(BeNil())
			})

			It("gets json list of provider templates", func() {
				res := doRequest("/api/v0/admin/provider_templates")
				Expect(res.Code).To(Equal(http.StatusOK))

				var ptsJson struct {
					Pts []*serializers.ProviderTemplate `json:"provider_templates"`
				}

				err := json.NewDecoder(res.Body).Decode(&ptsJson)
				Expect(err).To(BeNil())

				pts := ptsJson.Pts
				Expect(pts).To(HaveLen(2))

				pt1, pt2 := pts[0], pts[1]

				Expect(pt1.Name).To(Equal("do-512MB"))
				Expect(pt1.Provider).To(Equal("digitalocean"))
				Expect(pt1.Config["foo"]).To(Equal("bar"))

				Expect(pt2.Name).To(Equal("do-1GB"))
				Expect(pt2.Provider).To(Equal("ec2"))
			})

			It("Get individual provider template", func() {
				res := doRequest(fmt.Sprintf("/api/v0/admin/provider_templates/%d", insertedPt1.ID))
				Expect(res.Code).To(Equal(http.StatusOK))

				ptJson := &struct {
					Pt *serializers.ProviderTemplate `json:"provider_template"`
				}{}

				err := json.NewDecoder(res.Body).Decode(&ptJson)
				Expect(err).To(BeNil())

				pt := ptJson.Pt

				Expect(pt.Name).To(Equal("do-512MB"))
				Expect(pt.Provider).To(Equal("digitalocean"))
				Expect(pt.Config["foo"]).To(Equal("bar"))
			})

			It("Update provider templates", func() {
				jsonForm := `{
					"provider_template": {
						"hourly_price": 100,
						"config": {
							"bar": "baz"
						}
					}
				}`

				req, err := NewBodyRequest("POST",
					fmt.Sprintf("/api/v0/admin/provider_templates/%d", insertedPt1.ID),
					jsonForm)
				Expect(err).To(BeNil())

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+t.Token)

				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusOK))

				pt1, err := providertemplate.FindByID(db, insertedPt1.ID)
				Expect(err).To(BeNil())

				Expect(pt1.Name).To(Equal("do-512MB"))
				Expect(pt1.Provider).To(Equal("digitalocean"))
				Expect(pt1.Config.Map["bar"].String).To(Equal("baz"))
			})
		})
	})
})
