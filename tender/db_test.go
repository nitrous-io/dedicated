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

	Describe("findHost", func() {
		It("returns the host with the provided name", func() {
			host, _ := findHost(db, "oldapp")
			Expect(host.IP.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name")
			Expect(host.Port).To(Equal(10001), "Expect to get port given a valid name")

			_, err := findHost(db, "invalid name")
			Expect(err).ToNot(BeNil(), "Expect to get err when name is invalid")
		})
	})

	Describe("findContainer", func() {
		It("return the container with the provided name", func() {
			container, _ := findContainer(db, "rails-app")
			Expect(container.Slug).To(Equal("rails-app"), "Expect to get correct container")
			Expect(container.HostSlug).ToNot(BeEmpty(), "Expect to get the host slug")
			Expect(container.HostIP.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name")
			Expect(container.HostPort).To(Equal(10001), "Expect to get port given a valid name")
			Expect(container.HostSecret).ToNot(BeEmpty(), "Expect to get host secret")

			_, err := findContainer(db, "invalid name")
			Expect(err).ToNot(BeNil(), "Expect to get err when name is invalid")
		})
	})
})
