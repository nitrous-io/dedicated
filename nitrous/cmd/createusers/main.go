package main

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/coupon"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

const Usage = `Usage: createusers <users csv file> [--live-run]
  <file> is a 2-column csv file of email, username
  Use --live-run to actually send create users, otherwise it will be a dry run
`

var (
	dbConn *db.Database
	dryrun = true

	dryrunErr = fmt.Errorf("Dryrun rollback")
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func main() {
	requiredArgs := 1
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

	envvars.CheckDB()
	var err error
	dbConn, err = db.NewDatabase(envvars.NitrousPgUrl)
	if err != nil {
		logrus.Fatalf("Error: %s", err)
	}

	csvPath := os.Args[1]
	rows, err := parseCSV(csvPath)
	if err != nil {
		logrus.Fatalf("Error parsing csv: %s", err)
	}

	for _, row := range rows {
		email, username := row[0], row[1]
		logrus.Debugf("Creating (%s, %s)", email, username)
		if err := createUser(email, username); err != nil {
			if err != dryrunErr {
				logrus.Errorf("Error creating user (email %s): %s", email, err)
			}
			continue
		}
	}
}

// creates user
// confirms user
// populates reset password token
// FIXME: inserts one-month coupon for user (this is one-off, removing it makes the command generic)
func createUser(email, username string) error {
	return dbConn.WithTransaction(func(tx *sqlx.Tx) error {
		randomPass, err := securerandom.Base64(32)
		if err != nil {
			return err
		}
		u := &user.User{
			Email:    email,
			Username: username,
			Password: randomPass,
		}

		if err := u.Validate(); err != nil {
			return err
		}

		logrus.Infof("Inserting user (%s, %s)", u.Email, u.Username)

		if err := u.Insert(tx); err != nil {
			return err
		}

		if err := u.SetResetPasswordToken(tx); err != nil {
			return err
		}

		// use microseconds as indicator that this user was auto-created
		// instead of via signing up
		// To find, use
		// `SELECT * FROM users WHERE extract(milliseconds from confirmed_at) = 123.456;`
		if _, err := tx.Exec(`
			UPDATE users
				SET confirmed_at = date_trunc('minute', now()) + interval '123456 microseconds'
				WHERE id = $1
		`, u.ID); err != nil {
			return err
		}

		// give longer grace period for resetting
		if _, err := tx.Exec(`
			UPDATE users
				SET reset_password_sent_at = now() + interval '3 days'
				WHERE id = $1
		`, u.ID); err != nil {
			return err
		}

		c := &coupon.Coupon{
			UserID: u.ID,
			Code:   "lite.100.month", // corresponds with models/coupon#mapping
		}

		if err := c.Insert(tx); err != nil {
			return err
		}

		if dryrun {
			// return error on dryrun so transaction is rolled back
			return dryrunErr
		} else {
			return nil
		}
	})
}

func parseCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = 2
	if _, err := r.Read(); err != nil { // headers
		return nil, err
	}
	return r.ReadAll()
}
