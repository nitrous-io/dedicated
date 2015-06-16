package main

// healthcheck command
// * hits nitrousd server healthcheck endpoint using the PROVISIONER_USER service account
// * NOTE: keep this silent on success (intended to be used in cronjob with MAILTO),
// * output healthcheck response if failed

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

var (
	dbConn *db.Database
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatal(envvars.DBEnvVar, "is not defined")
	}

	if envvars.WebHostname == "" {
		logrus.Fatal("WEB_HOSTNAME is not defined")
	}

	// used as user that hits healthcheck url
	pu := envvars.ProvisionerUser
	if pu == "" {
		logrus.Fatal("healthcheck: PROVISIONER_USER is not defined")
	}

	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("healthcheck: db error: %s", err)
	}

	u, err := user.FindByUsername(dbConn, pu)
	if err != nil {
		logrus.Fatalf("healthcheck: %s user not found: %s", pu, err)
	}

	t, err := token.FindByUser(dbConn, u.ID)
	if err != nil {
		logrus.Fatalf("healthcheck: error on token for user %d: %s", u.ID, err)
	}

	url := fmt.Sprintf("https://%s/api/v0/admin/amazon/healthcheck", envvars.WebHostname)
	c := httpclient.New(t.Token)
	resp, err := c.GetAuthed(url, nil)
	if err != nil {
		logrus.Fatalf("healthcheck: endpoint error: %v", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logrus.Fatalf("healthcheck: got non-200 error response: %d; error reading body: %s", resp.StatusCode, err)
		}

		logrus.Fatalf("healthcheck: got non-200 error response: %d - %s", resp.StatusCode, string(body))
	}

	var rjson struct {
		Healthcheck map[string]interface{} `json:"healthcheck"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rjson); err != nil {
		logrus.Fatalf("healthcheck: error decoding json: %s", err)
	}

	h := rjson.Healthcheck
	if p, ok := h["passed"]; ok {
		if passed, ok := p.(bool); ok && passed {
			os.Exit(0)
		}
	}
	logparts := []string{"Healthcheck failed"}
	for k, v := range h {
		logparts = append(logparts, fmt.Sprintf("%s: %v", k, v))
	}
	logrus.Error(strings.Join(logparts, "; "))
	os.Exit(1)
}
