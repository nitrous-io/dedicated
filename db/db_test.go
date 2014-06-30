package db_test

import (
	"os/exec"
	"testing"

	"github.com/nitrous-io/nitrous/db"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "db")
}

var _ = Describe("Database", func() {
	var d *db.Database

	BeforeEach(func() {
		err := exec.Command("dropdb", "--if-exists", "nitrous_migrations_test").Run()
		Expect(err).To(BeNil())
		err = exec.Command("createdb", "-E", "UTF8", "-T", "template0", "-l", "en_US.utf8", "nitrous_migrations_test").Run()
		Expect(err).To(BeNil())
		d, err = ConnectToTestDatabase("nitrous_migrations_test")
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
				var newID int
				err = d.QueryRow(`SELECT id FROM people WHERE name = 'Peter'`).Scan(&newID)
				Expect(err).To(BeNil())
				Expect(newID).To(Equal(id))
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
