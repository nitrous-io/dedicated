package plans_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/plan_segment"
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

	doRequest := func(url string, auth bool) {
		req, err := NewRequest("GET", url, nil)
		Expect(err).To(BeNil())

		if auth {
			req.Header.Set("Authorization", "Bearer secret")
		}

		handler.ServeHTTP(res, req)
	}

	fetchPlans := func(url string, auth bool) []string {
		doRequest(url, auth)
		Expect(res.Code).To(Equal(http.StatusOK))

		j := struct {
			Plans []*serializers.Plan `json:"plans"`
		}{}
		err := json.NewDecoder(res.Body).Decode(&j)
		Expect(err).To(BeNil())

		return mapPlansToSlugs(j.Plans)
	}

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
		Context("when plans exist", func() {
			var (
				userDefaultSegmentID     int64
				freeUserDefaultSegmentID int64
			)
			plans := []*plan.Plan{
				{Slug: "free", Price: 0},
				{Slug: "start", Price: 500},
				{Slug: "shift", Price: 1000},
				{Slug: "turbo", Price: 1500},
				{Slug: "obsolete", Price: 2000, Disabled: true},
			}

			BeforeEach(func() {
				// Get default segment ID
				err := db.QueryRow(`SELECT id from segments where user_default='t'`).Scan(&userDefaultSegmentID)
				Expect(err).NotTo(HaveOccurred())

				err = db.QueryRow(`SELECT id from segments where free_user_default='t'`).Scan(&freeUserDefaultSegmentID)
				Expect(err).NotTo(HaveOccurred())

				r := region.Region{
					Slug: "sfo",
					Name: "US West",
				}
				Expect(r.Insert(db)).To(BeNil())

				for _, pl := range plans {
					Expect(pl.Insert(db)).To(BeNil())

					// Add plan segments
					if pl.Slug != "free" {
						ps := plan_segment.PlanSegment{SegmentID: freeUserDefaultSegmentID, PlanID: pl.ID}
						Expect(ps.Insert(db)).NotTo(HaveOccurred())
					}
					ps := plan_segment.PlanSegment{SegmentID: userDefaultSegmentID, PlanID: pl.ID}
					Expect(ps.Insert(db)).NotTo(HaveOccurred())
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
				// Add plan segments
				ps := plan_segment.PlanSegment{SegmentID: freeUserDefaultSegmentID, PlanID: pl.ID}
				Expect(ps.Insert(db)).NotTo(HaveOccurred())
				ps = plan_segment.PlanSegment{SegmentID: userDefaultSegmentID, PlanID: pl.ID}
				Expect(ps.Insert(db)).NotTo(HaveOccurred())
			})

			Context("When user is not logged in", func() {
				It("should show all plans", func() {
					plans := fetchPlans("/api/v0/plans", false)
					Expect(plans).To(ConsistOf([]string{"free", "start", "shift", "turbo", "shift-always-on"}))
				})
			})

			Context("when user is paid user", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET state='paid' WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())

					_, err = db.Exec(`UPDATE users SET segment_id=$1 WHERE id=$2;`, freeUserDefaultSegmentID, u.ID)
					Expect(err).To(BeNil())
				})

				It("should return paid plans of the default provider", func() {
					plans := fetchPlans("/api/v0/plans", true)
					Expect(plans).To(ConsistOf([]string{"start", "shift", "turbo", "shift-always-on"}))
				})

				It("should return all plans if no query parameter is provided", func() {
					plans := fetchPlans("/api/v0/plans", true)
					Expect(plans).To(ConsistOf([]string{"start", "shift", "turbo", "shift-always-on"}))
				})
			})

			Context("when user is in free tier", func() {
				BeforeEach(func() {
					u.SegmentID = userDefaultSegmentID
					err := u.Update(db)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return free plans of the default provider", func() {
					plans := fetchPlans("/api/v0/plans", true)
					Expect(plans).To(ConsistOf([]string{"free", "start", "shift", "turbo", "shift-always-on"}))
				})
			})
		})

		It("returns empty plans array when no plans exist", func() {
			doRequest("/api/v0/plans", true)
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				Plans []serializers.Plan `json:"plans"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Plans).To(HaveLen(0))
		})
	})

	Describe("GET /api/v0/orgs/{org_slug}/plans", func() {
		var (
			orgDefaultSegmentID  int64
			userDefaultSegmentID int64
			o                    *org.Org
		)

		BeforeEach(func() {
			plans := []*plan.Plan{
				{Slug: "plus", Price: 6400},
				{Slug: "pro", Price: 12800},
			}

			// Get default segment ID
			err := db.QueryRow(`SELECT id from segments where org_default='t'`).Scan(&orgDefaultSegmentID)
			Expect(err).NotTo(HaveOccurred())

			err = db.QueryRow(`SELECT id from segments where user_default='t'`).Scan(&userDefaultSegmentID)
			Expect(err).NotTo(HaveOccurred())

			for _, pl := range plans {
				Expect(pl.Insert(db)).To(BeNil())

				ps := plan_segment.PlanSegment{SegmentID: orgDefaultSegmentID, PlanID: pl.ID}
				Expect(ps.Insert(db)).NotTo(HaveOccurred())
			}

			// Add a plan that uses another plan as its base plan.
			userPlan := &plan.Plan{Slug: "Basic", Price: 900}
			Expect(userPlan.Insert(db)).NotTo(HaveOccurred())
			ps := plan_segment.PlanSegment{SegmentID: userDefaultSegmentID, PlanID: userPlan.ID}
			Expect(ps.Insert(db)).NotTo(HaveOccurred())

			// Add org
			o = &org.Org{
				Name:      "Test",
				Email:     "test@nitrous.io",
				Slug:      "test_org",
				SegmentID: orgDefaultSegmentID,
			}
			Expect(o.Insert(db)).To(BeNil())

			// Add current user to org
			err = orgmembership.AddMember(db, o.ID, u.ID, "admin")
			Expect(err).To(BeNil())
		})

		It("returns org plans", func() {
			plans := fetchPlans("/api/v0/orgs/test_org/plans", true)
			Expect(plans).To(ConsistOf([]string{"plus", "pro"}))
		})

		It("works with org ID", func() {
			url := fmt.Sprintf("/api/v0/orgs/%d/plans", o.ID)
			plans := fetchPlans(url, true)
			Expect(plans).To(ConsistOf([]string{"plus", "pro"}))
		})

		Context("org does not exist", func() {
			It("returns org plans", func() {
				doRequest("/api/v0/orgs/no_org/plans", true)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("user does not belong to org", func() {
			BeforeEach(func() {
				err := orgmembership.RemoveMember(db, o.ID, u.ID)
				Expect(err).To(BeNil())
			})

			It("returns an error", func() {
				doRequest("/api/v0/orgs/test_org/plans", true)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})
	})
})
