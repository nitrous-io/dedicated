package plans_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/nitrous-common/logger"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/plans")
}

var _ = Describe("controllers/plans", func() {
	var (
		db      *db.Database
		res     *httptest.ResponseRecorder
		handler *mux.Router
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr := NewTestServer("", db, logger, NewFakeTracker())

		handler = svr.Router()
		res = httptest.NewRecorder()

		u := &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		t := &token.Token{
			UserID: u.ID,
			Token:  "secret",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/plans", func() {
		doRequest := func() {
			req, err := NewRequest("GET", "/api/v0/plans", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer secret")
			handler.ServeHTTP(res, req)
		}

		Context("when plans exist", func() {
			var planSlugs = []string{"start", "shift", "turbo"}

			BeforeEach(func() {
				r := region.Region{
					Slug: "sfo",
					Name: "US West",
				}
				Expect(r.Insert(db)).To(BeNil())

				for _, n := range planSlugs {
					pl := plan.Plan{Slug: n, Price: 500}
					Expect(pl.Insert(db)).To(BeNil())
					t := providertemplate.ProviderTemplate{
						PlanID:   pl.ID,
						RegionID: r.ID,
						Provider: envvars.DefaultProvider,
					}
					Expect(t.Insert(db)).To(BeNil())
				}

				// Add a disabled plan - it should not be returned.
				pl := plan.Plan{Slug: "super-turbo", Disabled: true}
				Expect(pl.Insert(db)).To(BeNil())
				t := providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: r.ID,
					Provider: envvars.DefaultProvider,
				}
				Expect(t.Insert(db)).To(BeNil())

				// Add a plan belonging to another provider - it should not be returned.
				pl = plan.Plan{Slug: "start-windows-azure"}
				Expect(pl.Insert(db)).To(BeNil())
				t = providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: r.ID,
					Provider: "windows-azure",
				}
				Expect(t.Insert(db)).To(BeNil())
			})

			It("returns valid plans of the default provider", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Plans []serializers.Plan `json:"plans"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Plans).To(HaveLen(3))
				slugs := make([]string, 0, len(planSlugs))
				for _, pl := range j.Plans {
					slugs = append(slugs, pl.Slug)
				}
				Expect(slugs).To(ConsistOf(planSlugs))
			})
		})

		It("returns empty plans array when no plans exist", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				Plans []serializers.Plan `json:"plans"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Plans).To(HaveLen(0))
		})
	})
})
