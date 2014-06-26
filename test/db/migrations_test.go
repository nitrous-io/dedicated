package dbtest

import (
	"fmt"
	"os/exec"
	"os/user"

	"github.com/nitrous-io/nitrous/db"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Database", func() {
	var d *db.Database

	BeforeEach(func() {
		var err error
		u, err := user.Current()
		err = exec.Command("dropdb", "--if-exists", "nitrous_migrations_test").Run()
		Expect(err).To(BeNil())
		err = exec.Command("createdb", "-E", "UTF8", "-T", "template0", "-l", "en_US.utf8", "nitrous_migrations_test").Run()
		Expect(err).To(BeNil())
		dburl := fmt.Sprintf("host=/var/run/postgresql user=%s dbname=nitrous_migrations_test sslmode=disable", u.Username)
		Expect(err).To(BeNil())
		d, err = db.NewDatabase(dburl)
		d.SetMaxIdleConns(0)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if d != nil {
			d.Close()
		}
	})

	Describe("NewSqliteDatabase()", func() {
		It("returns *Database with usable sqlite db handle", func() {
			Expect(d).NotTo(BeNil())
			Expect(d).NotTo(BeNil())
			var ans int
			err := d.QueryRow("SELECT 1+2").Scan(&ans)
			Expect(err).To(BeNil())
			Expect(ans).To(Equal(3))
		})
	})

	Describe("Migrate()", func() {
		var (
			migrations = []*db.Migration{
				{Version: "10", Up: `CREATE TABLE people (
					id serial PRIMARY KEY NOT NULL,
					name character varying(255)
				)`},
				{Version: "20", Up: `ALTER TABLE people ADD COLUMN age integer`},
				{Version: "30", Up: `ALTER TABLE people ADD COLUMN birthyear integer`},
			}
		)

		getCols := func() []string {
			rows, err := d.Query(`SELECT * FROM people`)
			defer rows.Close()
			Expect(err).To(BeNil())
			cols, err := rows.Columns()
			Expect(err).To(BeNil())
			return cols
		}

		Context("when no migration has run yet", func() {
			It("should not have the table created", func() {
				_, err := d.Exec(`SELECT * FROM people`)
				Expect(err).NotTo(BeNil()) // no such table
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
				_, err := d.Exec(`INSERT INTO people (name) VALUES ('Peter')`)
				Expect(err).To(BeNil())
				var id int
				err = d.QueryRow(`SELECT id FROM people WHERE name = 'Peter'`).Scan(&id)
				Expect(err).To(BeNil())
				Expect(id).NotTo(BeZero())
				doAction()
				var newId int
				err = d.QueryRow(`SELECT id FROM people WHERE name = 'Peter'`).Scan(&newId)
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
