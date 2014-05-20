package dockertest

import (
	"fmt"
	"os"

	"github.com/nitrous-io/freighter/datastore"
	"github.com/nitrous-io/freighter/provider/docker"
	. "github.com/nitrous-io/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PortPool", func() {
	var (
		store  *datastore.Datastore
		dbFile string
		pool   *docker.PortPool
	)

	BeforeEach(func() {
		var err error
		dbFile = TempFile("sqlite3")
		store, err = datastore.NewSqliteDatastore(dbFile)
		Expect(err).To(BeNil())

		for i := 60001; i < 60004; i++ {
			err = store.Set(fmt.Sprintf("port:%v", i), "0")
			Expect(err).To(BeNil())
		}

		pool = docker.NewPortPool(store)
	})

	AfterEach(func() {
		if store != nil && store.Conn != nil {
			store.Conn.Close()
		}
		os.Remove(dbFile)
	})

	Describe("Acquire()", func() {
		Context("when there is a free port", func() {
			BeforeEach(func() {
				err := store.Set("port:60001", "1")
				Expect(err).To(BeNil())
			})

			It("returns first free port", func() {
				port, err := pool.Acquire()
				Expect(err).To(BeNil())
				Expect(port).To(Equal(uint16(60002)))
				port, err = pool.Acquire()
				Expect(err).To(BeNil())
				Expect(port).To(Equal(uint16(60003)))
			})
		})

		Context("when all ports are being used", func() {
			BeforeEach(func() {
				for i := 60001; i < 60004; i++ {
					err := store.Set(fmt.Sprintf("port:%v", i), "1")
					Expect(err).To(BeNil())
				}
			})

			It("returns error", func() {
				port, err := pool.Acquire()
				Expect(port).To(BeZero())
				Expect(err).NotTo(BeNil())
				Expect(err).To(MatchError(docker.ErrNoFreePort))
			})
		})
	})

	Describe("Release()", func() {
		Context("when a given port is already free", func() {
			It("does nothing", func() {
				err := pool.Release(60001)
				Expect(err).To(BeNil())
				val, err := store.Get("port:60001")
				Expect(err).To(BeNil())
				Expect(val).To(Equal("0"))
			})
		})

		Context("when a given port is being used", func() {
			BeforeEach(func() {
				err := store.Set("port:60001", "1")
				Expect(err).To(BeNil())
			})

			It("marks it as free", func() {
				err := pool.Release(60001)
				Expect(err).To(BeNil())
				val, err := store.Get("port:60001")
				Expect(err).To(BeNil())
				Expect(val).To(Equal("0"))
			})
		})

		Context("when a port not in the port pool is given", func() {
			It("does nothing", func() {
				err := pool.Release(12345)
				Expect(err).To(BeNil())
				val, err := store.Get("port:12345")
				Expect(err).To(BeNil())
				Expect(val).To(Equal(""))
			})
		})
	})
})
