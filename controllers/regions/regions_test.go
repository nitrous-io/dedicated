package regions_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/regions")
}

var _ = Describe("controllers/regions", func() {
	var (
		db              *db.Database
		svr             *server.Server
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = u.Insert(db)
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

	Describe("GET /api/v0/regions", func() {
		doRequest := func() {
			req, err := NewAuthedRequest(t, "GET", "/api/v0/regions", nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		Context("when regions exist", func() {
			var regionNames = []string{"uswest", "useast", "europe"}

			BeforeEach(func() {
				for _, n := range regionNames {
					r := region.Region{Name: n}
					Expect(r.Insert(db)).To(BeNil())
				}
			})

			It("returns serialized regions in json", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Regions []serializers.Region `json:"regions"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Regions).To(HaveLen(3))
				names := []string{}
				for _, r := range j.Regions {
					names = append(names, r.Name)
				}
				Expect(names).To(ConsistOf(regionNames))
			})
		})

		It("returns empty regions array when no regions exist", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				Regions []serializers.Region `json:"regions"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Regions).To(HaveLen(0))
		})
	})
})
