package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/Sirupsen/logrus"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
)

const Usage = `Usage: hostmassterminate <csv file> <user id> [--live-run]
  <csv file> is a csv file with a header, has a single column for host slugs
  <user id> is the user to run as
`

var (
	dbConn *db.Database
	dryrun = true
	userID int64
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func main() {
	requiredArgs := 2
	for _, arg := range os.Args {
		if arg == "--live-run" {
			dryrun = false
			requiredArgs += 1
		}
	}
	if len(os.Args) != requiredArgs+1 {
		fmt.Println(Usage)
		logrus.Fatal("Wrong number of arguments")
	}

	csvPath, userIDStr := os.Args[1], os.Args[2]
	userIDInt, _ := strconv.Atoi(userIDStr)
	if userIDInt == 0 {
		logrus.Fatalf("Failed to parse user id %s", os.Args[2])
	}
	userID = int64(userIDInt)

	envvars.CheckDB()
	var err error
	dbConn, err = db.NewDatabase(envvars.NitrousPgUrl)
	if err != nil {
		logrus.Fatal(err)
	}

	rows, err := parseCSV(csvPath)
	if err != nil {
		logrus.Fatalf("Error parsing csv: %s", err)
	}

	t := getToken()

	for _, row := range rows {
		if err := terminateHost(row[0], t); err != nil {
			logrus.Fatal(err)
		}
	}
}

func getToken() string {
	if dryrun {
		return "dryrun token"
	}
	t := &token.Token{UserID: userID}

	if err := t.Insert(dbConn); err != nil {
		logrus.Fatalf("Error generating token %v", err)
	}
	logrus.Debugf("Created token id %d", t.ID)
	return t.Token
}

func terminateHost(slug, tkn string) error {
	logrus.Debugf("Destroying host slug %s", slug)
	if dryrun {
		return nil
	}
	u := fmt.Sprintf("https://%s/api/v0/hosts/%s", envvars.WebHostname, slug)
	client := http.DefaultClient
	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tkn))
	resp, err := client.Do(req)
	if err != nil {
		logrus.Errorf("Error DELETEing %s for host %s, err: %v", u, slug, err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var j map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
			return err
		}
		return fmt.Errorf("Unsuccessful: %+v", j)
	}
	return nil
}

func parseCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = 1
	if _, err := r.Read(); err != nil { // headers
		return nil, err
	}
	return r.ReadAll()
}
