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

	Describe("hostIP", func() {
		It("returns hosts with the provided name", func() {
			ip, port, _ := hostIP(db, "oldapp")
			Expect(ip.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name")
			Expect(port).To(Equal(10001), "Expect to get port given a valid name")

			_, _, err := hostIP(db, "invalid name")
			Expect(err).ToNot(BeNil(), "Expect to get err when name is invalid")
		})
	})
})
