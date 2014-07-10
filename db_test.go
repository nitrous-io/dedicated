package tender

import (
	"database/sql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("db", func() {
	var (
		db *sql.DB
	)

	BeforeEach(func() {
		db = prepareDB()
		loadFixtures(db)
	})

	Describe("user", func() {
		It("returns user id of the access token", func() {
			var err error
			_, err = user(db, "token")
			Expect(err).To(BeNil(), "Expect to get user from valid token")

			_, err = user(db, "missing_user")
			Expect(err).ToNot(BeNil(), "Expect to get err when user does not exist")

			_, err = user(db, "invalid token")
			Expect(err).ToNot(BeNil(), "Expect to get err when token is invalid")
		})
	})

	Describe("host", func() {
		It("returns hosts with the provided name", func() {
			ip, _ := host(db, "nitrous")
			Expect(ip.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name")

			_, err := host(db, "invalid name")
			Expect(err).ToNot(BeNil(), "Expect to get err when name is invalid")
		})
	})

	Describe("hostByUser", func() {
		It("returns hosts belongs to an user", func() {
			user, _ := user(db, "token")

			ip, _ := hostByUser(db, user, "nitrous")
			Expect(ip.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name and owner")

			_, err := hostByUser(db, user, "invalid name")
			Expect(err).ToNot(BeNil(), "Expect to get err when user doesnt have the host")
		})
	})
})
