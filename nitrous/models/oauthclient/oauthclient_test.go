package oauthclient_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/oauthclient")
}

var _ = Describe("models/oauthclient", func() {
	var (
		db            *db.Database
		u, clientUser *user.User
		client        *oauthclient.Client
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "demoman",
			Password: "123456",
			Email:    "demoman@mann.co",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		clientUser = &user.User{
			Username: "blutarch",
			Password: "mannup",
			Email:    "info@blutarch.com",
		}
		err = WrapInTransaction(db, clientUser.Insert)
		Expect(err).To(BeNil())
		Expect(clientUser.ID).NotTo(BeZero())

		client = &oauthclient.Client{
			UserID:      clientUser.ID,
			Name:        "Builders League United",
			Description: "A fictional company",
			ClientID:    "blu_team",
			RedirectURI: "https://blu.co/oauth/callback",
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("InsertWithSecret()", func() {
		It("should insert to db correctly with secret", func() {
			err := client.InsertWithSecret(db, "secret_sauce")
			Expect(err).To(BeNil())

			client2 := &oauthclient.ClientWithSecret{}

			err = db.Get(client2, `SELECT *
				FROM oauth_clients WHERE user_id=$1;
			`, clientUser.ID)
			Expect(err).To(BeNil())

			Expect(client2.Name).To(Equal("Builders League United"))
			Expect(client2.Description).To(Equal("A fictional company"))
			Expect(client2.ClientID).To(Equal("blu_team"))
			Expect(client2.RedirectURI).To(Equal("https://blu.co/oauth/callback"))
			Expect(client2.ClientSecret).To(Equal("secret_sauce"))
		})
	})

	Describe("FindByID()", func() {
		BeforeEach(func() {
			err := client.InsertWithSecret(db, "secret_sauce")
			Expect(err).To(BeNil())
		})

		It("should find client result by client id", func() {
			c, err := oauthclient.FindByID(db, "blu_team")
			Expect(err).To(BeNil())

			Expect(c.ID).To(Equal(client.ID))
			Expect(c.ClientID).To(Equal("blu_team"))
			Expect(c.UserID).To(Equal(clientUser.ID))
		})

		It("should find client result by client id and secret", func() {
			c, err := oauthclient.FindWithSecret(db, "blu_team", "secret_sauce")
			Expect(err).To(BeNil())

			Expect(c.ID).To(Equal(client.ID))
			Expect(c.ClientID).To(Equal("blu_team"))
			Expect(c.UserID).To(Equal(clientUser.ID))
		})

		It("should not find client with wrong secret", func() {
			_, err := oauthclient.FindWithSecret(db, "blu_team", "redspy")
			Expect(err).ToNot(BeNil())
		})
	})
})
