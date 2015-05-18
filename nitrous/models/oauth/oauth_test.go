package oauth_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/oauth"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/msgverifier"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/oauth")
}

var _ = Describe("controllers/oauth", func() {
	var (
		db            *db.Database
		u, clientUser *user.User
		client        *oauth.Client
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
	})

	BeforeEach(func() {
		u = &user.User{
			Username: "demoman",
			Password: "123456",
			Email:    "demoman@mann.co",
		}
		err := WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())
	})

	BeforeEach(func() {
		clientUser = &user.User{
			Username: "blutarch",
			Password: "mannup",
			Email:    "info@blutarch.com",
		}
		err := WrapInTransaction(db, clientUser.Insert)
		Expect(err).To(BeNil())
		Expect(clientUser.ID).NotTo(BeZero())

		client = &oauth.Client{
			UserID:      clientUser.ID,
			Name:        "Builders League United",
			Description: "A fictional company",
			ClientID:    "blu_team",
			RedirectURI: "https://blu.co/oauth/callback",
		}
		err = client.InsertWithSecret(db, "secret_sauce")
		Expect(err).To(BeNil())
	})

	generateCode := func() string {
		signedCode, err := oauth.GenerateAuthorizationCode(u.ID, "blu_team", "https://blu.co/oauth/callback")
		Expect(err).To(BeNil())

		return signedCode
	}

	Describe("client model", func() {
		It("should find client result by client id", func() {
			c, err := oauth.FindClientByID(db, "blu_team")
			Expect(err).To(BeNil())

			Expect(c.ID).To(Equal(client.ID))
			Expect(c.ClientID).To(Equal("blu_team"))
			Expect(c.UserID).To(Equal(clientUser.ID))
		})

		It("should find client result by client id and secret", func() {
			c, err := oauth.FindClientWithSecret(db, "blu_team", "secret_sauce")
			Expect(err).To(BeNil())

			Expect(c.ID).To(Equal(client.ID))
			Expect(c.ClientID).To(Equal("blu_team"))
			Expect(c.UserID).To(Equal(clientUser.ID))
		})

		It("should not find client with wrong secret", func() {
			_, err := oauth.FindClientWithSecret(db, "blu_team", "redspy")
			Expect(err).ToNot(BeNil())
		})
	})

	Describe("authorization code", func() {
		It("should be able to read generated code", func() {
			signedCode := generateCode()
			Expect(signedCode).ToNot(Equal(""))

			parsedCode, err := oauth.ReadAuthorizationCode(signedCode)
			Expect(err).To(BeNil())

			Expect(parsedCode.Type).To(Equal("authorization_code"))
			Expect(parsedCode.UserID).To(Equal(u.ID))
			Expect(parsedCode.ClientID).To(Equal("blu_team"))
			Expect(parsedCode.RedirectURI).To(Equal("https://blu.co/oauth/callback"))
			Expect(parsedCode.Nonce).ToNot(Equal(""))
		})

		It("should not able to read code signed with different secret", func() {
			createdAt := time.Now()
			unsignedCode := oauth.AuthorizationCode{
				Type:        "authorization_code",
				UserID:      u.ID,
				ClientID:    client.ClientID,
				Nonce:       "not_so_random",
				RedirectURI: "https://red.co/oauth/callback",
				CreatedAt:   createdAt,
				ExpiresAt:   createdAt.Add(999 * time.Hour),
			}

			signedCode, err := msgverifier.New("redspy").Generate(unsignedCode)
			Expect(err).To(BeNil())

			_, err = oauth.ReadAuthorizationCode(signedCode)
			Expect(err).To(Equal(oauth.ErrInvalidToken))
		})

		It("should not able to read code after expired", func() {
			// modify global to get a short-lived code
			originalValidity := oauth.ValidityPeriod
			oauth.ValidityPeriod = 500 * time.Millisecond
			signedCode := generateCode()
			oauth.ValidityPeriod = originalValidity

			time.Sleep(1 * time.Second)

			_, err := oauth.ReadAuthorizationCode(signedCode)
			Expect(err).To(Equal(oauth.ErrCodeExpired))
		})
	})
})
