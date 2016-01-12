package regions_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/regions")
}

var _ = Describe("controllers/regions", func() {
	var (
		db  *db.Database
		svr *server.Server
		res *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = NewTestServer("", db, nil)
		res = httptest.NewRecorder()
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/regions", func() {
		doRequest := func() {
			req, err := NewRequest("GET", "/api/v0/regions", nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		Context("when regions exist", func() {
			var regionSlugs = []string{"uswest", "useast", "europe"}

			BeforeEach(func() {
				pl := plan.Plan{Slug: "Shift"}
				Expect(pl.Insert(db)).To(BeNil())

				for _, n := range regionSlugs {
					r := region.Region{Slug: n}
					Expect(r.Insert(db)).To(BeNil())
					t := providertemplate.ProviderTemplate{
						PlanID:   pl.ID,
						RegionID: r.ID,
						Provider: envvars.DefaultProvider,
					}
					Expect(t.Insert(db)).To(BeNil())
				}

				// Add a region belonging to another provider - it should not be returned.
				r := region.Region{Slug: "azure-redmond"}
				Expect(r.Insert(db)).To(BeNil())
				t := providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: r.ID,
					Provider: "windows-azure",
				}
				Expect(t.Insert(db)).To(BeNil())
			})

			It("returns valid regions of the default provider", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Regions []serializers.Region `json:"regions"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Regions).To(HaveLen(3))
				slugs := make([]string, 0, len(regionSlugs))
				for _, r := range j.Regions {
					slugs = append(slugs, r.Slug)
				}
				Expect(slugs).To(ConsistOf(regionSlugs))
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
