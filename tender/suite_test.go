package tender

import (
	"bytes"
	"database/sql"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"
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

func runPsqlMust(dbName, sqlFile string) {
	args := []string{
		"-d", dbName,
		"-f", sqlFile,
	}
	var stderr bytes.Buffer
	cmd := exec.Command("psql", args...)
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
	// running psql doesn't return err when there are problems in *.sql file
	// so we have to check for existence of "ERROR:" in stderr output
	stderrStr := strings.TrimSpace(string(stderr.Bytes()))
	if -1 != strings.Index(stderrStr, "ERROR:") {
		logrus.Errorf("psql failed with stderr: '%s'", stderrStr)
		panic(fmt.Errorf("psql failed to import %s", sqlFile))
	}
}

func loadFixtures() {
	runPsqlMust("tender_test", "./testdata/fixtures.sql")
}
