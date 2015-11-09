package plans_test

import (
	"database/sql"
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

		svr := NewTestServer("", db, NewFakeTracker())

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
		doRequest := func(url string, auth bool) {
			req, err := NewRequest("GET", url, nil)
			Expect(err).To(BeNil())

			if auth {
				req.Header.Set("Authorization", "Bearer secret")
			}

			handler.ServeHTTP(res, req)
		}

		fetchPlans := func(url string, auth bool) []string {
			doRequest("/api/v0/plans?eligible=true", auth)
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				Plans []*serializers.Plan `json:"plans"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			return mapPlansToSlugs(j.Plans)
		}

		Context("when plans exist", func() {
			plans := []*plan.Plan{
				{Slug: "free", Price: 0},
				{Slug: "start", Price: 500},
				{Slug: "shift", Price: 1000},
				{Slug: "turbo", Price: 1500},
				{Slug: "obsolete", Price: 2000, Disabled: true},
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

				// Add a plan that uses another plan as its base plan.
				shiftPlan, err := plan.FindBySlug(db, "shift")
				Expect(err).To(BeNil())
				pl := plan.Plan{
					BasePlanID: sql.NullInt64{shiftPlan.ID, true},
					Slug:       "shift-always-on",
					Price:      2500,
					Hidden:     true,
				}
				Expect(pl.Insert(db)).To(BeNil())
				pt := providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: r.ID,
					Provider: envvars.DefaultProvider,
				}
				Expect(pt.Insert(db)).To(BeNil())

				// Add a plan belonging to another provider - it should not be returned.
				pl = plan.Plan{Slug: "start-windows-azure", Price: 3000}
				Expect(pl.Insert(db)).To(BeNil())
				pt = providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: r.ID,
					Provider: "windows-azure",
				}
				Expect(pt.Insert(db)).To(BeNil())
			})

			Context("When user is not logged in", func() {
				It("should show all plans", func() {
					plans := fetchPlans("/api/v0/plans?eligible=true", false)
					Expect(plans).To(ConsistOf([]string{"free", "start", "shift", "turbo", "shift-always-on"}))
				})
			})

			Context("when user is paid user", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET state='paid' WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())
				})

				It("should return paid plans of the default provider", func() {
					plans := fetchPlans("/api/v0/plans?eligible=true", true)
					Expect(plans).To(ConsistOf([]string{"start", "shift", "turbo", "shift-always-on"}))
				})

				It("should return all plans if no query parameter is provided", func() {
					plans := fetchPlans("/api/v0/plans", true)
					Expect(plans).To(ConsistOf([]string{"start", "shift", "turbo", "shift-always-on"}))
				})
			})

			Context("when user is in free tier", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET state='free' WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())

					_, err = db.Exec(`UPDATE users SET phone_authed = true WHERE id = $1;`, u.ID)
					Expect(err).To(BeNil())
				})

				It("should return free plans of the default provider", func() {
					plans := fetchPlans("/api/v0/plans?eligible=true", true)
					Expect(plans).To(ConsistOf([]string{"free", "start", "shift", "turbo", "shift-always-on"}))
				})

				Context("When user has created a free-tier host", func() {
					BeforeEach(func() {
						_, err := CreateHostStack(db, &HostStack{
							U: u,
							// first plan fixture is free plan,
							// but it doesn't really matter for this test
							P: plans[0],
						})
						Expect(err).To(BeNil())
					})

					It("should return only paid plans", func() {
						plans := fetchPlans("/api/v0/plans?eligible=true", true)
						Expect(plans).To(ConsistOf([]string{"start", "shift", "turbo", "shift-always-on"}))
					})
				})
			})
		})

		It("returns empty plans array when no plans exist", func() {
			doRequest("/api/v0/plans?eligible=true", true)
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
