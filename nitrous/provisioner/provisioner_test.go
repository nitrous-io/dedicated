package provisioner_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/nitrous-common/logger"

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
		lgr             *logger.Logger
		provisionerUser *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		lgr = &logger.Logger{Level: logger.LogLevelQuiet}

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
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)

			err = prov.Start()
			Expect(err).To(BeNil())
			defer prov.Stop()

			Eventually(func() int {
				count, err := host.CountPreprovisioned(db, hs.R, hs.P)
				Expect(err).To(BeNil())
				return count
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(1))
		})

		It("assigns hosts instead of preprovisioning if they are available with no bypass", func() {
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
					Price: 100,
				},
			})
			Expect(err).To(BeNil())

			Expect(hs.H.HourlyPrice).To(Equal(int64(0)))
			Expect(hs.H.Free).To(Equal(true))

			count, err := host.CountPreprovisioned(db, hs.R, hs.P)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1), "Pre-condition: 1 pre-provisioned hosts.")

			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)
			err = prov.Start()
			Expect(err).To(BeNil())
			defer prov.Stop()

			_, err = db.Exec("UPDATE user_profiles SET max_hosts = 99 WHERE user_id = $1", hs.U.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE billing_profiles SET state = 'paid' WHERE user_id = $1", hs.U.ID)
			Expect(err).To(BeNil())

			canCreate, err := hs.U.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(canCreate).To(BeTrue())

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
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(false))
		})

		It("bypasses preprovisioned if option given even if host is available", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{Preprovisioned: true},
				PT: &providertemplate.ProviderTemplate{
					PreprovisionMax: 1,
					Provider:        "docker",
				},
			})
			Expect(err).To(BeNil())
			count, err := host.CountPreprovisioned(db, hs.R, hs.P)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1), "Pre-condition: 1 pre-provisioned hosts.")

			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)
			err = prov.Start()
			Expect(err).To(BeNil())
			defer prov.Stop()

			_, err = db.Exec("UPDATE user_profiles SET max_hosts = 99 WHERE user_id = $1", hs.U.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE billing_profiles SET state = 'paid' WHERE user_id = $1", hs.U.ID)
			Expect(err).To(BeNil())

			canCreate, err := hs.U.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(canCreate).To(BeTrue())

			newHost, err := prov.ProvisionHost(provisioner.ProvisionOptions{
				OutputStream:       GinkgoWriter,
				Slug:               "bypassed",
				Name:               "new-name",
				User:               hs.U,
				Region:             hs.R,
				Plan:               hs.P,
				Template:           hs.PT,
				UUID:               "DEF",
				BypassPreprovision: true,
			})
			Expect(err).To(BeNil())
			Expect(newHost.ID).NotTo(Equal(hs.H.ID))

			// assert post config data
			var hcfg hostbuilderclient.Config
			Expect(rt.Requests).To(HaveLen(1))
			req := rt.Requests[0]
			json.Unmarshal([]byte(req.FormValue("config")), &hcfg)
			Expect(hcfg["host_id"]).To(Equal(float64(newHost.ID)))
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
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)

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
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)

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
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)

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
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)
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
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)
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

	Describe("assigning a pre-provisioned host to user", func() {
		It("updates the host in Hostbuilder and the host record with info from Hostbuilder", func() {
			// Replace provisioner's hostbuilder client with a fake version.
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: `
				{"stream":"starting: creating host"}
				{"stream":"finished: creating host"}
				{"id":"abcd1234", "ip_address":"1.2.3.4", "provider":"dummy", "freighterd_port":65333}`,
				Status: http.StatusOK,
			}
			hb.Client.Client.Transport = rt
			provisioner.HostbuilderClient = hb

			// Setup provisioner so that we have a pre-provisioned host.
			hs, err := test.CreateHostStack(db, &test.HostStack{
				PT: &providertemplate.ProviderTemplate{PreprovisionMax: 1, Provider: "dummy"},
			})
			Expect(err).To(BeNil())
			cfg := &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 10 * time.Millisecond,
			}
			prov := provisioner.NewProvisioner(db, lgr, nil, cfg)
			err = prov.Start()
			Expect(err).To(BeNil())

			// Wait for host to be pre-provisioned.
			provisioned := make(chan struct{})
			errCh := make(chan error)
			go func() {
				ticker := time.NewTicker(10 * time.Millisecond)
				for _ = range ticker.C {
					count, err := host.CountPreprovisioned(db, hs.R, hs.P)
					if err != nil {
						errCh <- err
						return
					}
					if count > 0 {
						provisioned <- struct{}{}
						return
					}
				}
			}()

			select {
			case <-provisioned:
				break
			case err := <-errCh:
				Fail(fmt.Sprintf("Error waiting for host to be pre-provisioned: %v", err))
			case <-time.After(5 * time.Second):
				Fail("Timed out waiting for host to be pre-provisioned")
			}

			u := &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err = test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			t := &token.Token{
				UserID: u.ID,
				Token:  "valid-token",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE billing_profiles SET state = 'paid' WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			// Fake update response from Hostbuilder.
			rt = &test.FakeRoundTripper{
				Message: `
					{"stream":"starting: updating host"}
					{"stream":"finished: updating host"}
					{"id":"abcd1234", "ip_address":"1.2.3.4", "provider":"dummy", "freighterd_port":65333}`,
				Status: http.StatusOK,
			}
			hb.Client.Client.Transport = rt

			preprovisionedHost, err := host.FindPreprovisioned(db, hs.R, hs.P)
			Expect(err).To(BeNil())

			// Actually provision the host so that it tries to assign the
			// pre-provisioned host to the user.
			_, err = prov.ProvisionHost(provisioner.ProvisionOptions{
				OutputStream: GinkgoWriter,
				Slug:         "new-slug",
				Name:         "new-name",
				User:         u,
				Region:       hs.R,
				Plan:         hs.P,
				Template:     hs.PT,
				UUID:         "ABC",
			})
			Expect(err).To(BeNil())

			// assert generated host
			h, err := host.Find(db, u, "new-slug")
			Expect(err).To(BeNil())
			Expect(h.Name.String).To(Equal("new-name"))
			Expect(h.ProviderIdentifier.String).To(Equal("abcd1234"))
			Expect(h.IPAddress.String).To(Equal("1.2.3.4"))
			Expect(h.FreighterdPort.Int64).To(Equal(int64(65333)))
			// The user's host should be the same as the pre-provisioned host.
			Expect(h.ID).To(Equal(preprovisionedHost.ID))

			// assert post config data
			Expect(len(rt.Requests) >= 1).To(BeTrue(), "failed to update the host")
			req := rt.Requests[0]
			Expect(req.Header.Get(constants.NitrousUUIDHeader)).To(Equal("ABC"))

			var hcfg hostbuilderclient.Config
			json.Unmarshal([]byte(req.FormValue("config")), &hcfg)
			Expect(hcfg["host_id"]).To(Equal(float64(h.ID)))
			Expect(hcfg["name"]).To(Equal("new-slug"))
			Expect(hcfg["apt_username"]).To(Equal(strconv.FormatInt(h.ID, 10)))
			Expect(hcfg["apt_password"]).To(Equal(h.Secret))
		})
	})

	Describe("ApplyProviderConfig()", func() {
		var (
			pcfg *provisioner.Config
			prov *provisioner.Provisioner
			hs   *test.HostStack
		)

		BeforeEach(func() {
			pcfg = &provisioner.Config{
				Username: provisionerUser.Username,
				Interval: 50 * time.Millisecond,
			}
			prov = provisioner.NewProvisioner(db, lgr, nil, pcfg)
			var err error
			hs, err = test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
		})

		It("uses convention for provider name", func() {
			hs.PT.Provider = provisioner.AmazonProvider
			origCfg := hostbuilderclient.Config{
				"region_id":    "usw2",
				"storage_size": "25",
			}

			cfg, err := prov.ApplyProviderConfig(origCfg, hs.PT, hs.H)
			Expect(err).To(BeNil())
			Expect(cfg["name"]).To(Equal(fmt.Sprintf("userhost-{{env}}-%d", hs.H.ID)))
		})

		It("amazon provider returns config given valid opts", func() {
			hs.PT.Provider = provisioner.AmazonProvider
			origCfg := hostbuilderclient.Config{
				"region_id":    "usw2",
				"storage_size": "25",
			}

			cfg, err := prov.ApplyProviderConfig(origCfg, hs.PT, hs.H)
			Expect(err).To(BeNil())
			Expect(cfg["storage_size"]).To(Equal(25))
			Expect(cfg["disable_eip"]).To(Equal(false))
		})

		It("amazon provider disables eip if set", func() {
			hs.PT.Provider = provisioner.AmazonProvider
			origCfg := hostbuilderclient.Config{
				"region_id":    "usw2",
				"storage_size": "25",
				"disable_eip":  "true",
			}

			cfg, err := prov.ApplyProviderConfig(origCfg, hs.PT, hs.H)
			Expect(err).To(BeNil())
			Expect(cfg["disable_eip"]).To(Equal(true))
		})

		It("amazon provider errs on invalid storage_size", func() {
			hs.PT.Provider = provisioner.AmazonProvider
			origCfg := hostbuilderclient.Config{
				"region_id":    "usw2",
				"storage_size": "mongo hdd",
			}

			_, err := prov.ApplyProviderConfig(origCfg, hs.PT, hs.H)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("invalid storage_size"))
		})
	})
})
