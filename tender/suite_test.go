package tender

import (
	"database/sql"
	"fmt"
	"os/exec"
	"os/user"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "server")
}

func prepareDB() *sql.DB {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	dbURL := fmt.Sprintf("host=/var/run/postgresql user=%s dbname=%s sslmode=disable",
		u.Username, "tender_test")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		panic(err)
	}
	if err := db.Ping(); err != nil {
		panic(err)
	}
	return db
}

func loadFixtures(db *sql.DB) {
	args := []string{
		"-d", "tender_test",
		"-f", "./testdata/fixtures.sql",
	}
	err := exec.Command("psql", args...).Run()
	if err != nil {
		panic(err)
	}
}
