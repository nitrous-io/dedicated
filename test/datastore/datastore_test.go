package datastoretest

import (
	"os"

	"github.com/nitrous-io/freighter/datastore"
	. "github.com/nitrous-io/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Datastore", func() {
	var (
		store  *datastore.Datastore
		dbFile string
	)

	BeforeEach(func() {
		var err error
		dbFile = TempFile("sqlite3")
		store, err = datastore.NewSqliteDatastore(dbFile)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if store != nil && store.Conn != nil {
			store.Conn.Close()
		}
		os.Remove(dbFile)
	})

	Describe("NewSqliteDatastore()", func() {
		It("returns *Datastore with usable sqlite db handle", func() {
			Expect(store).NotTo(BeNil())
			Expect(store.Conn).NotTo(BeNil())
			var ans int
			err := store.Conn.QueryRow("SELECT 1+2").Scan(&ans)
			Expect(err).To(BeNil())
			Expect(ans).To(Equal(3))
		})

		It(`creates "store" table`, func() {
			rows, err := store.Conn.Query("SELECT * FROM store")
			Expect(err).To(BeNil())
			cols, err := rows.Columns()
			Expect(err).To(BeNil())
			Expect(cols).To(Equal([]string{"key", "val"}))

			_, err = store.Conn.Exec(`INSERT INTO store (key, val) VALUES ("foo", "bar")`)
			Expect(err).To(BeNil())
		})
	})

	Describe("*Datastore", func() {
		BeforeEach(func() {
			for _, q := range []string{
				`INSERT INTO store (key, val) VALUES ("user:300", "foo")`,
				`INSERT INTO store (key, val) VALUES ("user:100", "foo")`,
				`INSERT INTO store (key, val) VALUES ("item:100", "bar")`,
				`INSERT INTO store (key, val) VALUES ("user:200", "baz")`,
			} {
				_, err := store.Conn.Exec(q)
				Expect(err).To(BeNil())
			}
		})

		Describe(".Set()", func() {
			BeforeEach(func() {
				err := store.Set("foo", "bar")
				Expect(err).To(BeNil())
				err = store.Set("baz", "qux")
				Expect(err).To(BeNil())
			})

			It("adds a given key-value pair to the datastore", func() {
				var val string
				err := store.Conn.QueryRow("SELECT val FROM store WHERE key = ?", "foo").Scan(&val)
				Expect(err).To(BeNil())
				Expect(val).To(Equal("bar"))
				err = store.Conn.QueryRow("SELECT val FROM store WHERE key = ?", "baz").Scan(&val)
				Expect(err).To(BeNil())
				Expect(val).To(Equal("qux"))
			})

			It("updates an existing key-value pair in the datastore", func() {
				err := store.Set("foo", "baz")
				Expect(err).To(BeNil())
				var val string
				err = store.Conn.QueryRow("SELECT val FROM store WHERE key = ?", "foo").Scan(&val)
				Expect(err).To(BeNil())
				Expect(val).To(Equal("baz"))
			})
		})

		Describe(".Update()", func() {
			Context("when a key-value pair with a given key does not exist", func() {
				It("does not create a new key-value pair", func() {
					err := store.Update("foo", "bar")
					Expect(err).To(BeNil())
					var exists bool
					err = store.Conn.QueryRow("SELECT EXISTS (SELECT 1 FROM store WHERE key = ?)", "foo").Scan(&exists)
					Expect(err).To(BeNil())
					Expect(exists).To(BeFalse())
				})
			})

			Context("when a key-value pair with a given key already exists", func() {
				It("updates an existing key-value pair in the datastore", func() {
					err := store.Update("user:100", "123456")
					Expect(err).To(BeNil())
					var val string
					err = store.Conn.QueryRow("SELECT val FROM store WHERE key = ?", "user:100").Scan(&val)
					Expect(err).To(BeNil())
					Expect(val).To(Equal("123456"))
				})
			})
		})

		Describe(".Get", func() {
			It("returns the value associated with a given key from the datastore", func() {
				val, err := store.Get("user:100")
				Expect(err).To(BeNil())
				Expect(val).To(Equal("foo"))
				val, err = store.Get("item:100")
				Expect(err).To(BeNil())
				Expect(val).To(Equal("bar"))
			})

			Context("when the key is not found", func() {
				It("returns empty string", func() {
					val, err := store.Get("user:999")
					Expect(err).To(BeNil())
					Expect(val).To(Equal(""))
				})
			})
		})

		Describe(".Find", func() {
			It("returns the key-value pairs with keys that match a given pattern, ordered by key", func() {
				kvs, err := store.Find("user:%")
				Expect(err).To(BeNil())
				Expect(kvs).To(Equal([]*datastore.KeyVal{
					{Key: "user:100", Val: "foo"},
					{Key: "user:200", Val: "baz"},
					{Key: "user:300", Val: "foo"},
				}))
			})

			Context("when no key-value pair has a key that matches a given pattern", func() {
				It("returns empty slice", func() {
					kvs, err := store.Find("computer:%")
					Expect(err).To(BeNil())
					Expect(kvs).To(Equal([]*datastore.KeyVal{}))
				})
			})
		})

		Describe(".FindOne", func() {
			It("returns the first key-value pair (ordered by key) with a key that matches a given pattern", func() {
				kv, err := store.FindOne("user:%")
				Expect(err).To(BeNil())
				Expect(kv).To(Equal(&datastore.KeyVal{Key: "user:100", Val: "foo"}))
			})

			Context("when no key-value pair has a key that matches a given pattern", func() {
				It("returns nil", func() {
					kv, err := store.FindOne("computer:%")
					Expect(err).To(BeNil())
					Expect(kv).To(BeNil())
				})
			})
		})

		Describe(".FindWhere", func() {
			It("returns the key-value pairs with keys that match a given pattern and values that equal to a given value, orderd by key", func() {
				kvs, err := store.FindWhere("user:%", "foo")
				Expect(err).To(BeNil())
				Expect(kvs).To(Equal([]*datastore.KeyVal{
					{Key: "user:100", Val: "foo"},
					{Key: "user:300", Val: "foo"},
				}))
			})

			Context("when no key-value pair has a key that matches a given pattern", func() {
				It("returns empty slice", func() {
					kvs, err := store.FindWhere("user:%", "bar")
					Expect(err).To(BeNil())
					Expect(kvs).To(Equal([]*datastore.KeyVal{}))
				})
			})
		})

		Describe(".FindOneWhere", func() {
			It("returns the first key-value pair (ordered by key) with a key that matches a given pattern", func() {
				kv, err := store.FindOneWhere("user:%", "foo")
				Expect(err).To(BeNil())
				Expect(kv).To(Equal(&datastore.KeyVal{Key: "user:100", Val: "foo"}))
			})

			Context("when no key-value pair has a key that matches a given pattern", func() {
				It("returns nil", func() {
					kv, err := store.FindOneWhere("user:%", "bar")
					Expect(err).To(BeNil())
					Expect(kv).To(BeNil())
				})
			})
		})
	})
})
