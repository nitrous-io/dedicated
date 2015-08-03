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
		loadFixtures()
		RefreshHostsCache(db)
		RefreshContainersCache(db)
	})

	Describe("host cache", func() {
		It("test host caching", func() {
			var err error

			host, ok := hostCacheGet("oldapp")
			Expect(ok).To(BeTrue())
			Expect(host.IP.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name")
			Expect(host.Port).To(Equal(10001), "Expect to get port given a valid name")

			// not in cache, returns nil
			host, ok = hostCacheGet("invalid name")
			Expect(ok).To(BeFalse())
			Expect(host).To(BeNil())

			host, err = hostGet(db, "invalid name")
			Expect(err).To(Equal(errHostNotFound))
			Expect(host).To(BeNil())

			// cached failed result, returns hostNotFound
			host, ok = hostCacheGet("invalid name")
			Expect(ok).To(BeTrue())
			Expect(host).To(Equal(hostNotFound))
		})

		It("marks removed hosts as stopped hosts", func() {
			var err error

			host, ok := hostCacheGet("oldapp")
			Expect(ok).To(BeTrue())
			Expect(host.IP.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name")
			Expect(host.Port).To(Equal(10001), "Expect to get port given a valid name")

			// Termiates hosts
			_, err = db.Exec("UPDATE hosts SET terminated_at = now() WHERE slug = 'oldapp'")
			Expect(err).To(BeNil())

			RefreshHostsCache(db)

			// Save as stopped or deleted container
			host, ok = hostCacheGet("oldapp")
			Expect(ok).To(BeTrue())
			Expect(host).To(Equal(hostNotFound))

			// Untermiates container
			_, err = db.Exec("UPDATE hosts SET terminated_at = NULL WHERE slug = 'oldapp'")
			Expect(err).To(BeNil())

			RefreshHostsCache(db)

			// Cache as live container
			host, ok = hostCacheGet("oldapp")
			Expect(ok).To(BeTrue())
			Expect(host.IP.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name")
			Expect(host.Port).To(Equal(10001), "Expect to get port given a valid name")
		})
	})

	Describe("container cache", func() {
		It("test container caching", func() {
			var err error

			container, ok := containerCacheGet("rails-app")
			Expect(ok).To(BeTrue())
			Expect(container.Slug).To(Equal("rails-app"), "Expect to get correct container")
			Expect(container.HostSlug).ToNot(BeEmpty(), "Expect to get the host slug")
			Expect(container.HostIP.String()).To(Equal("10.0.0.1"), "Expect to get host ip given a valid name")
			Expect(container.HostPort).To(Equal(10001), "Expect to get port given a valid name")
			Expect(container.HostSecret).ToNot(BeEmpty(), "Expect to get host secret")

			// not in cache, returns nil
			container, ok = containerCacheGet("invalid name")
			Expect(ok).To(BeFalse())
			Expect(container).To(BeNil())

			container, err = containerGet(db, "invalid name")
			Expect(err).To(Equal(errContainerNotFound))
			Expect(container).To(BeNil())

			// not in cache, returns containerNotFound
			container, ok = containerCacheGet("invalid name")
			Expect(ok).To(BeTrue())
			Expect(container).To(Equal(containerNotFound))
		})

		It("marks removed containers as stopped containers", func() {
			var err error

			container, ok := containerCacheGet("rails-app")
			Expect(ok).To(BeTrue())

			// Termiates container
			_, err = db.Exec("UPDATE containers SET terminated_at = now() WHERE slug = 'rails-app'")
			Expect(err).To(BeNil())

			RefreshContainersCache(db)

			// Save as stopped or deleted container
			container, ok = containerCacheGet("rails-app")
			Expect(ok).To(BeTrue())
			Expect(container).To(Equal(containerNotFound))

			// Untermiates container
			_, err = db.Exec("UPDATE containers SET terminated_at = NULL WHERE slug = 'rails-app'")
			Expect(err).To(BeNil())

			RefreshContainersCache(db)

			// Cache as live container
			container, ok = containerCacheGet("rails-app")
			Expect(ok).To(BeTrue())
			Expect(container.Slug).To(Equal("rails-app"), "Expect to get correct container")
		})
	})
})
