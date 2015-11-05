package provisioner_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/lib/pq/hstore"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	// shorten keylen so tests run faster
	cert.KeyLen = 512
	keypairs.KeyLen = 512
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "provisioner")
}

var _ = Describe("provisioner", func() {
	var (
		db              *ndb.Database
		provisionerUser *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		provisionerUser = &user.User{
			Username: "host-provisioner",
			Email:    "admin@nitrous.io",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, provisionerUser.Insert)
		Expect(err).To(BeNil())

		provisionerUser.ServiceAccount = true
		_, err = db.Exec("UPDATE users SET service_account = true WHERE id = $1;", provisionerUser.ID)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Start()", func() {
		var rt *test.FakeRoundTripper

		BeforeEach(func() {
			// Replace provisioner's hostbuilder client with a fake version.
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt = &test.FakeRoundTripper{
				Message: `
				{"stream":"starting: creating host"}
				{"stream":"finished: creating host"}
				{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"docker", "freighterd_port":65333}`,
				Status: http.StatusOK,
			}
			hb.Client.Client.Transport = rt
			provisioner.HostbuilderClient = hb
		})

		It("pre-provisions hosts when required", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				PT: &providertemplate.ProviderTemplate{
					PreprovisionMax: 1,
					Provider:        "docker",
					Config:          hstore.Hstore{Map: map[string]sql.NullString{"region_id": {"sgp", true}}},
				},
			})
			Expect(err).To(BeNil())
			count, err := host.CountPreprovisioned(db, hs.R, hs.P)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0), "Pre-condition: 0 pre-provisioned hosts.")

			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, nil, cfg)

			// Actual provisioning part will be done in queue handler.
			// This it just to make sure that it does not occur any errors.
			err = prov.Start()
			Expect(err).To(BeNil())
			defer prov.Stop()
		})

		It("assigns hosts instead of preprovisioning if they are available", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Preprovisioned: true,
					HourlyPrice:    0,
					Free:           true,
				},
				PT: &providertemplate.ProviderTemplate{
					PreprovisionMax: 1,
					Provider:        "docker",
				},
				P: &plan.Plan{
					Price:        100,
					AutoShutdown: true,
				},
			})
			Expect(err).To(BeNil())

			Expect(hs.H.HourlyPrice).To(Equal(int64(0)))
			Expect(hs.H.Free).To(Equal(true))
			Expect(hs.P.AutoShutdown).To(Equal(true))

			count, err := host.CountPreprovisioned(db, hs.R, hs.P)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1), "Pre-condition: 1 pre-provisioned hosts.")

			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, nil, cfg)
			err = prov.Start()
			Expect(err).To(BeNil())
			defer prov.Stop()

			_, err = db.Exec("UPDATE user_profiles SET max_hosts = 99 WHERE user_id = $1", hs.U.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE billing_profiles SET state = 'paid' WHERE user_id = $1", hs.U.ID)
			Expect(err).To(BeNil())

			err = hs.U.CanCreateHosts(db)
			Expect(err).To(BeNil())

			_, err = prov.ProvisionHost(provisioner.ProvisionOptions{
				OutputStream: GinkgoWriter,
				Slug:         "new-slug",
				Name:         "new-name",
				User:         hs.U,
				Region:       hs.R,
				Plan:         hs.P,
				Template:     hs.PT,
				UUID:         "ABC",
			})
			Expect(err).To(BeNil())

			// assert post config data
			var hcfg hostbuilderclient.Config
			Expect(rt.Requests).To(HaveLen(1))
			req := rt.Requests[0]
			json.Unmarshal([]byte(req.FormValue("config")), &hcfg)
			Expect(hcfg["host_id"]).To(Equal(float64(hs.H.ID)))

			Eventually(func() int {
				count, err := host.CountPreprovisioned(db, hs.R, hs.P)
				Expect(err).To(BeNil())
				return count
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(0))

			Eventually(func() int64 {
				h, err := host.FindByIdentifier(db, "new-slug")
				if err != nil {
					return 0
				}
				return h.HourlyPrice
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(hs.P.HourlyPrice()))

			Eventually(func() bool {
				h, err := host.FindByIdentifier(db, "new-slug")
				if err != nil {
					return true
				}
				return h.Free
			}, 5*time.Second, 500*time.Millisecond).Should(BeFalse())

			Eventually(func() bool {
				h, err := host.FindByIdentifier(db, "new-slug")
				if err != nil {
					return false
				}
				return h.AutoShutdown
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
		})

		It("does not pre-provision hosts in a region that is disabled", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				PT: &providertemplate.ProviderTemplate{PreprovisionMax: 1, Provider: "docker"},
			})
			Expect(err).To(BeNil())
			count, err := host.CountPreprovisioned(db, hs.R, hs.P)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0)) // Pre-condition: 0 pre-provisioned hosts.

			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, nil, cfg)

			_, err = db.Exec("UPDATE regions SET disabled_at = now() WHERE id = $1", hs.R.ID)
			Expect(err).To(BeNil())

			err = prov.Start()
			Expect(err).To(BeNil())

			Consistently(func() int {
				count, err := host.CountPreprovisioned(db, hs.R, hs.P)
				Expect(err).To(BeNil())
				return count
			}, 1*time.Second).Should(Equal(0))
		})

		It("does not pre-provision hosts in a region that has orphan pre-provisioned hosts", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				H:  &host.Host{State: host.StateOrphan, Preprovisioned: true},
				PT: &providertemplate.ProviderTemplate{PreprovisionMax: 1},
			})
			count, err := host.CountPreprovisionedOrphans(db, hs.R)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1)) // Pre-condition: > 0 orphan pre-provisioned hosts.

			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, nil, cfg)

			err = prov.Start()
			Expect(err).To(BeNil())

			Consistently(func() int {
				count, err := host.CountPreprovisioned(db, hs.R, hs.P)
				Expect(err).To(BeNil())
				return count
			}, 1*time.Second).Should(Equal(0))
		})

		It("returns error on failure to find provisioner user", func() {
			cfg := &provisioner.Config{
				Username: "no such user",
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, nil, cfg)

			err := prov.Start()
			Expect(err).To(MatchError("User 'no such user' could not be found"))
		})
	})

	Describe("Stop()", func() {
		It("stops the pre-provisioner", func() {
			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, nil, cfg)
			err := prov.Start()
			Expect(err).To(BeNil())

			Expect(prov.IsRunning()).To(BeTrue())

			prov.Stop()
			Expect(prov.IsRunning()).To(BeFalse())
		})

		It("allows pre-provisioner to be restarted with SIGUSR2", func() {
			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, nil, cfg)
			err := prov.Start()
			Expect(err).To(BeNil())

			prov.Stop()
			Expect(prov.IsRunning()).To(BeFalse())

			syscall.Kill(syscall.Getpid(), syscall.SIGUSR2)

			Eventually(func() bool {
				return prov.IsRunning()
			}).Should(BeTrue())
		})
	})
})
