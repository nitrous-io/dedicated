package dbtest

import (
	"fmt"
	"os"
	"os/user"

	"github.com/nitrous-io/nitrous/db"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Database", func() {
	var d *db.Database

	BeforeEach(func() {
		var err error
		dburl := os.Getenv("NITROUS_POSTGRES_URL_TEST")
		if dburl == "" {
			u, err := user.Current()
			dburl = fmt.Sprintf("host=/var/run/postgresql user=%s dbname=nitrous_test sslmode=disable", u.Username)
			Expect(err).To(BeNil())
		}
		d, err = db.NewDatabase(dburl)
		Expect(err).To(BeNil())
		d.DB.Exec("DROP TABLE schema_migrations CASCADE")
		d.DB.Exec("DROP TABLE people CASCADE")
	})

	AfterEach(func() {
		if d != nil && d.DB != nil {
			d.DB.Close()
		}
	})

	Describe("NewSqliteDatabase()", func() {
		It("returns *Database with usable sqlite db handle", func() {
			Expect(d).NotTo(BeNil())
			Expect(d.DB).NotTo(BeNil())
			var ans int
			err := d.DB.QueryRow("SELECT 1+2").Scan(&ans)
			Expect(err).To(BeNil())
			Expect(ans).To(Equal(3))
		})
	})

	Describe("Migrate()", func() {
		var (
			migrations = []*db.Migration{
				{Version: "0", Up: `CREATE TABLE people (
					id bigserial PRIMARY KEY NOT NULL,
					name character varying(255)
				)`},
				{Version: "1", Up: `ALTER TABLE people ADD COLUMN age integer`},
				{Version: "2", Up: `ALTER TABLE people ADD COLUMN birthyear integer`},
			}
		)

		getCols := func() []string {
			rows, err := d.DB.Query(`SELECT * FROM people`)
			Expect(err).To(BeNil())
			cols, err := rows.Columns()
			Expect(err).To(BeNil())
			return cols
		}

		It("runs all migrations", func() {
			Context("when no migration has run yet", func() {
				It("should not have the table created", func() {
					_, err := d.DB.Exec(`SELECT * FROM people`)
					Expect(err).NotTo(BeNil()) // no such table
				})
			})
		})

		Context("when some migration has been run", func() {
			BeforeEach(func() {
				err := d.Migrate(migrations[:1])
				Expect(err).To(BeNil())
			})

			doAction := func() {
				err := d.Migrate(migrations)
				Expect(err).To(BeNil())
			}

			It("runs migrations that have not been run", func() {
				Expect(getCols()).To(Equal([]string{"id", "name"}))
				doAction()
				Expect(getCols()).To(Equal([]string{"id", "name", "age", "birthyear"}))
			})

			It("does not affect data", func() {
				_, err := d.DB.Exec(`INSERT INTO people (name) VALUES ('Peter')`)
				Expect(err).To(BeNil())
				var id int
				err = d.DB.QueryRow(`SELECT id FROM people WHERE name = 'Peter'`).Scan(&id)
				Expect(err).To(BeNil())
				Expect(id).NotTo(BeZero())
				doAction()
				var newId int
				err = d.DB.QueryRow(`SELECT id FROM people WHERE name = 'Peter'`).Scan(&newId)
				Expect(err).To(BeNil())
				Expect(newId).To(Equal(id))
			})
		})

		Context("when all migrations have been run", func() {
			BeforeEach(func() {
				err := d.Migrate(migrations)
				Expect(err).To(BeNil())
			})

			It("does not run any more migrations", func() {
				Expect(getCols()).To(Equal([]string{"id", "name", "age", "birthyear"}))
				err := d.Migrate(migrations)
				Expect(err).To(BeNil())
				Expect(getCols()).To(Equal([]string{"id", "name", "age", "birthyear"}))
			})
		})
	})
})
