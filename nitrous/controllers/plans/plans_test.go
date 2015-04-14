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

func mapPlansToSlugs(plans []*serializers.Plan) []string {
	slugs := make([]string, len(plans))
	for i, plan := range plans {
		slugs[i] = plan.Slug
	}
	return slugs
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/plans")
}

var _ = Describe("controllers/plans", func() {
	var (
		db      *db.Database
		u       *user.User
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

		u = &user.User{
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
			plans := []plan.Plan{
				{
					Slug:  "free",
					Price: 0,
				},
				{
					Slug:  "start",
					Price: 500,
				},
				{
					Slug:  "shift",
					Price: 1000,
				},
				{
					Slug:  "turbo",
					Price: 1500,
				},
				{
					Slug:     "hidden",
					Price:    2000,
					Disabled: true,
				},
			}

			BeforeEach(func() {
				r := region.Region{
					Slug: "sfo",
					Name: "US West",
				}
				Expect(r.Insert(db)).To(BeNil())

				for _, pl := range plans {
					Expect(pl.Insert(db)).To(BeNil())
					pt := providertemplate.ProviderTemplate{
						PlanID:   pl.ID,
						RegionID: r.ID,
						Provider: envvars.DefaultProvider,
					}
					Expect(pt.Insert(db)).To(BeNil())
				}

				// Add a plan belonging to another provider - it should not be returned.
				pl := plan.Plan{Slug: "start-windows-azure", Price: 3000}
				Expect(pl.Insert(db)).To(BeNil())
				pt := providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: r.ID,
					Provider: "windows-azure",
				}
				Expect(pt.Insert(db)).To(BeNil())
			})

			Context("when user is paid user", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET state='paid' WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())
				})

				It("returns paid plans of the default provider", func() {
					doRequest()
					Expect(res.Code).To(Equal(http.StatusOK))

					j := struct {
						Plans []*serializers.Plan `json:"plans"`
					}{}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())

					Expect(mapPlansToSlugs(j.Plans)).To(ConsistOf([]string{"start", "shift", "turbo"}))
				})
			})

			Context("when user is in free trial", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET state='free' WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())
				})

				It("returns free plans of the default provider", func() {
					doRequest()
					Expect(res.Code).To(Equal(http.StatusOK))

					j := struct {
						Plans []*serializers.Plan `json:"plans"`
					}{}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())

					Expect(mapPlansToSlugs(j.Plans)).To(ConsistOf([]string{"free", "start", "shift", "turbo"}))
				})
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
