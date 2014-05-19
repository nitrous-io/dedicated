package dockertest

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/nitrous-io/freighter/provider/docker"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Database", func() {
	var (
		d      *docker.Database
		dbFile string
	)

	BeforeEach(func() {
		dbFile = tempDBFile()
	})

	AfterEach(func() {
		if d != nil && d.Db != nil {
			d.Db.Close()
		}
		os.Remove(dbFile)
	})

	Describe("NewSqliteDatabase()", func() {
		BeforeEach(func() {
			var err error
			d, err = docker.NewSqliteDatabase(dbFile)
			Expect(err).To(BeNil())
		})

		It("returns *Database with usable sqlite db handle", func() {
			Expect(d).NotTo(BeNil())
			Expect(d.Db).NotTo(BeNil())
			var ans int
			err := d.Db.QueryRow("SELECT 1+2").Scan(&ans)
			Expect(err).To(BeNil())
			Expect(ans).To(Equal(3))
		})
	})

	Describe("Migrate()", func() {
		var (
			migrations = [][]string{
				{"0", `CREATE TABLE people (
					id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
					name VARCHAR(20)
				)`},
				{"1", `ALTER TABLE people ADD COLUMN age INTEGER`},
				{"2", `ALTER TABLE people ADD COLUMN birthyear INTEGER`},
			}
		)

		BeforeEach(func() {
			var err error
			d, err = docker.NewSqliteDatabase(dbFile)
			Expect(err).To(BeNil())
		})

		getCols := func() []string {
			rows, err := d.Db.Query(`SELECT * FROM people`)
			Expect(err).To(BeNil())
			cols, err := rows.Columns()
			Expect(err).To(BeNil())
			return cols
		}

		Context("when no migration has run yet", func() {
			It("runs all migrations", func() {
				_, err := d.Db.Exec(`SELECT * FROM people`)
				Expect(err).NotTo(BeNil()) // no such table
				err = d.Migrate(migrations)
				Expect(err).To(BeNil())
				Expect(getCols()).To(Equal([]string{"id", "name", "age", "birthyear"}))
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
				_, err := d.Db.Exec(`INSERT INTO people (name) VALUES ("Peter")`)
				Expect(err).To(BeNil())
				var id int
				err = d.Db.QueryRow(`SELECT id FROM people WHERE name = "Peter"`).Scan(&id)
				Expect(err).To(BeNil())
				Expect(id).NotTo(BeZero())
				doAction()
				var newId int
				err = d.Db.QueryRow(`SELECT id FROM people WHERE name = "Peter"`).Scan(&newId)
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

func tempDBFile() string {
	return path.Join(os.TempDir(), fmt.Sprintf("freightertest-%v.sqlite3", time.Now().UnixNano()))
}
