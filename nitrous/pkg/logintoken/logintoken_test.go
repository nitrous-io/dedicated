package logintoken_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/pkg/logintoken"
	"github.com/nitrous-io/nitrous-common/msgverifier"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/logintoken")
}

var _ = Describe("pkg/logintoken", func() {
	It("generates login tokens that can be verified", func() {
		uid1, uid2 := int64(123), int64(456)

		tok, err := logintoken.Generate(uid1)
		Expect(err).To(BeNil())
		Expect(tok).NotTo(BeEmpty())

		tok2, err := logintoken.Generate(uid2)
		Expect(err).To(BeNil())
		Expect(tok2).NotTo(BeEmpty())

		id, err := logintoken.Read(tok)
		Expect(err).To(BeNil())
		Expect(id).To(Equal(uid1))

		id, err = logintoken.Read(tok2)
		Expect(err).To(BeNil())
		Expect(id).To(Equal(uid2))
	})

	It("is unique", func() {
		tok, err := logintoken.Generate(123)
		Expect(err).To(BeNil())
		Expect(tok).NotTo(BeEmpty())
		tok2, err := logintoken.Generate(123)
		Expect(err).To(BeNil())
		Expect(tok2).NotTo(BeEmpty())
		tok3, err := logintoken.Generate(123)
		Expect(err).To(BeNil())
		Expect(tok3).NotTo(BeEmpty())

		Expect(tok).NotTo(Equal(tok2))
		Expect(tok).NotTo(Equal(tok3))
		Expect(tok2).NotTo(Equal(tok3))
	})

	It("returns error if token is expired", func() {
		origPeriod := logintoken.DefaultValidityPeriod
		logintoken.DefaultValidityPeriod = 1 * time.Millisecond
		defer func() {
			logintoken.DefaultValidityPeriod = origPeriod
		}()

		tok, err := logintoken.Generate(123)
		Expect(err).To(BeNil())
		Expect(tok).NotTo(BeEmpty())

		time.Sleep(2 * time.Millisecond)
		id, err := logintoken.Read(tok)
		Expect(err).To(MatchError(logintoken.ErrTokenExpired))
		Expect(id).To(Equal(int64(0)))
	})

	It("returns error if token is signed using a different secret", func() {
		origSecret := logintoken.Secret
		logintoken.Secret = "F4k3 53cR37 y0"
		defer func() {
			logintoken.Secret = origSecret
		}()

		tok, err := logintoken.Generate(123)
		Expect(err).To(BeNil())
		Expect(tok).NotTo(BeEmpty())

		logintoken.Secret = origSecret
		id, err := logintoken.Read(tok)
		Expect(err).To(MatchError(logintoken.ErrInvalidToken))
		Expect(id).To(Equal(int64(0)))
	})

	It("returns error if token is not a login token", func() {
		createdAt := time.Now().UTC()
		expiresAt := createdAt.Add(1 * time.Hour)
		tok, err := msgverifier.New(logintoken.Secret).Generate(&struct {
			Type      string     `json:"type"`
			UserID    int64      `json:"user_id"`
			CreatedAt *time.Time `json:"created_at"`
			ExpiresAt *time.Time `json:"expires_at"`
		}{
			Type:      "authorizationcode",
			UserID:    123,
			CreatedAt: &createdAt,
			ExpiresAt: &expiresAt,
		})
		Expect(err).To(BeNil())

		id, err := logintoken.Read(tok)
		Expect(err).To(MatchError(logintoken.ErrInvalidToken))
		Expect(id).To(Equal(int64(0)))
	})
})
