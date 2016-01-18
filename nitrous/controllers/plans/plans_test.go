package plans_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"

	"github.com/nitrous-io/dedicated/nitrous/test"
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
		u       *user.User
		handler *mux.Router
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		svr := test.NewTestServer("", db, test.NewFakeTracker())
		handler = svr.Router()

		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

		t := &token.Token{
			UserID: u.ID,
			Token:  "secret",
		}
		Expect(t.Insert(db)).To(BeNil())

		plans := []*plan.Plan{
			{Slug: "free", Price: 0},
			{Slug: "start", Price: 500},
			{Slug: "shift", Price: 1000},
			{Slug: "turbo", Price: 1500},
			{Slug: "obsolete", Price: 2000, Disabled: true},
			{Slug: "shift-always-on", Price: 1200, Hidden: true},
		}

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

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/plans", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewRequest("GET", "/api/v0/plans", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer secret")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Context("when user is paid user", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state='paid' WHERE user_id=$1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("returns non-hidden, non-disabled, paid plans of the default provider", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Plans []*serializers.Plan `json:"plans"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				slugs := []string{}
				for _, p := range j.Plans {
					slugs = append(slugs, p.Slug)
				}
				Expect(slugs).To(ConsistOf([]string{"start", "shift", "turbo"}))
			})
		})

		Context("when user has no card in billing profile", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state='no_card' WHERE user_id=$1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("returns both free and paid plans (non-hidden, non-disabled)", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Plans []*serializers.Plan `json:"plans"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				slugs := []string{}
				for _, p := range j.Plans {
					slugs = append(slugs, p.Slug)
				}
				Expect(slugs).To(ConsistOf([]string{"free", "start", "shift", "turbo"}))
			})
		})

		Context("when user is in free tier", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state='free' WHERE user_id=$1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("returns both free and paid plans (non-hidden, non-disabled)", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Plans []*serializers.Plan `json:"plans"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				slugs := []string{}
				for _, p := range j.Plans {
					slugs = append(slugs, p.Slug)
				}
				Expect(slugs).To(ConsistOf([]string{"free", "start", "shift", "turbo"}))
			})

			It("returns both free and paid plans (non-hidden, non-disabled) when user has orphaned, non-terminated host", func() {
				p, err := plan.FindBySlug(db, "free")
				Expect(err).To(BeNil())

				hs, err := test.CreateHostStack(db, &test.HostStack{
					U: u,
					P: p,
				})
				Expect(err).To(BeNil())

				Expect(hs.H.TerminatedAt).To(BeNil())

				_, err = db.Exec(`UPDATE hosts SET state = $1 WHERE id = $2`, host.StateOrphan, hs.H.ID)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Plans []*serializers.Plan `json:"plans"`
				}{}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				slugs := []string{}
				for _, p := range j.Plans {
					slugs = append(slugs, p.Slug)
				}
				Expect(slugs).To(ConsistOf([]string{"free", "start", "shift", "turbo"}))
			})

			It("returns only non-hidden, non-disabled, paid plans when user has created a free-tier-host", func() {
				_, err := test.CreateHostStack(db, &test.HostStack{
					U: u,
					P: &plan.Plan{Slug: "freeplan", Price: 0},
				})
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Plans []*serializers.Plan `json:"plans"`
				}{}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				slugs := []string{}
				for _, p := range j.Plans {
					slugs = append(slugs, p.Slug)
				}
				Expect(slugs).To(ConsistOf([]string{"start", "shift", "turbo"}))
			})
		})
	})

	Describe("GET /api/v0/plans/all", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewRequest("GET", "/api/v0/plans/all", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should show all plans as long as they're not disabled or hidden", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				Plans []*serializers.Plan `json:"plans"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			slugs := []string{}
			for _, p := range j.Plans {
				slugs = append(slugs, p.Slug)
			}
			Expect(slugs).To(ConsistOf([]string{"free", "start", "shift", "turbo"}))
		})
	})
})
