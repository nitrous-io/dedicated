package authorizationcode_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/authorizationcode"
	"github.com/nitrous-io/nitrous-common/msgverifier"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/authorizationcode")
}

var _ = Describe("pkg/authorizationcode", func() {
	Describe("Generate()", func() {
		It("should generate token with correct fields", func() {
			signedCode, err := authorizationcode.Generate(123, "blu_team", "https://blu.co/oauth/callback")
			Expect(err).To(BeNil())

			parsedCode := &authorizationcode.Code{}
			verifier := msgverifier.New(envvars.SecretToken)
			err = verifier.Verify(signedCode, parsedCode)

			Expect(parsedCode.Type).To(Equal("authorization_code"))
			Expect(parsedCode.UserID).To(Equal(int64(123)))
			Expect(parsedCode.ClientID).To(Equal("blu_team"))
			Expect(parsedCode.Nonce).NotTo(BeEmpty())
			Expect(parsedCode.RedirectURI).To(Equal("https://blu.co/oauth/callback"))
			Expect(parsedCode.CreatedAt).To(BeTemporally("~", time.Now(), 1*time.Second))
		})
	})

	Describe("Read()", func() {
		It("should be able to read generated code", func() {
			signedCode, err := authorizationcode.Generate(123, "blu_team", "https://blu.co/oauth/callback")
			Expect(err).To(BeNil())
			Expect(signedCode).NotTo(BeEmpty())

			parsedCode, err := authorizationcode.Read(signedCode)
			Expect(err).To(BeNil())

			Expect(parsedCode.Type).To(Equal("authorization_code"))
			Expect(parsedCode.UserID).To(Equal(int64(123)))
			Expect(parsedCode.ClientID).To(Equal("blu_team"))
			Expect(parsedCode.RedirectURI).To(Equal("https://blu.co/oauth/callback"))
			Expect(parsedCode.Nonce).NotTo(BeEmpty())
		})

		It("should not able to read code signed with different secret", func() {
			createdAt := time.Now()
			unsignedCode := authorizationcode.Code{
				Type:        "authorization_code",
				UserID:      123,
				ClientID:    "abcdefg",
				Nonce:       "not_so_random",
				RedirectURI: "https://red.co/oauth/callback",
				CreatedAt:   createdAt,
				ExpiresAt:   createdAt.Add(999 * time.Hour),
			}

			signedCode, err := msgverifier.New("redspy").Generate(unsignedCode)
			Expect(err).To(BeNil())

			_, err = authorizationcode.Read(signedCode)
			Expect(err).To(Equal(authorizationcode.ErrInvalidToken))
		})

		It("should not able to read code after expired", func() {
			// modify global to get a negative-lived code
			originalValidity := authorizationcode.ValidityPeriod
			authorizationcode.ValidityPeriod = -5 * time.Second
			signedCode, err := authorizationcode.Generate(123, "blu_team", "https://blu.co/oauth/callback")
			Expect(err).To(BeNil())
			authorizationcode.ValidityPeriod = originalValidity

			_, err = authorizationcode.Read(signedCode)
			Expect(err).To(Equal(authorizationcode.ErrCodeExpired))
		})
	})
})
