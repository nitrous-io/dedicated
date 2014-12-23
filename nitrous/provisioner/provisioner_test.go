package provisioner_test

import (
	"net/http"
	"syscall"
	"testing"
	"time"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

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
	})

	Describe("Start()", func() {
		BeforeEach(func() {
			// Replace provisioner's hostbuilder client with a fake version.
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: `
				{"stream":"starting: creating host"}
				{"stream":"finished: creating host"}
				{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"dummy", "freighterd_port":65333}`,
				Status: http.StatusOK,
			}
			hb.Client.Client.Transport = rt
			provisioner.HostbuilderClient = hb
		})

		It("pre-provisions hosts when required", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				PT: &providertemplate.ProviderTemplate{PreprovisionMax: 1},
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

			err = prov.Start()
			Expect(err).To(BeNil())

			Eventually(func() int {
				count, err := host.CountPreprovisioned(db, hs.R, hs.P)
				Expect(err).To(BeNil())
				return count
			}, 5*time.Second).Should(Equal(1))
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
})
