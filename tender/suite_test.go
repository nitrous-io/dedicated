package tender

import (
	"database/sql"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "server")
}

func prepareDB() *sql.DB {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		panic("Missing DATABASE_URL")
	}
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
		"-d", "nitrous",
		"-f", "./testdata/fixtures.sql",
	}
	err := exec.Command("psql", args...).Run()
	if err != nil {
		panic(err)
	}
}
