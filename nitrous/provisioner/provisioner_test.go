package provisioner_test

import (
	"net/http"
	"syscall"
	"testing"
	"time"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
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
