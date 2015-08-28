package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

type SeedFn func(db.QueryReceiver) error

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatal(envvars.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatal(err)
	}
	err = dbConn.MigrateAll("")
	if err != nil {
		logrus.Infof("Migrations failed: %v", err)
	}
	defer dbConn.DB.Close()

	tx, err := dbConn.Beginx()
	if err != nil {
		logrus.Fatal(err)
	}

	err = createAdminUserAndToken(tx)
	if err != nil {
		tx.Rollback()
		logrus.Fatal(err)
	}

	err = tx.Commit()
	if err != nil {
		logrus.Fatal(err)
	}
}

func shouldCreateAdminUser(d db.QueryReceiver) (bool, error) {
	c := -1
	err := d.QueryRowx("SELECT COUNT(*) FROM users WHERE username = $1", "admin").Scan(&c)
	switch {
	case err != nil:
		return false, err
	case c > 0:
		logrus.Infof("%d found, no need to create admin user", c)
		return false, nil
	case c == 0:
		return true, nil
	default: // if it's still -1
		logrus.Errorf("Unexpected count %v", c)
		return false, err
	}
}

func createAdminUserAndToken(d db.QueryReceiver) error {
	ok, err := shouldCreateAdminUser(d)
	if err != nil || !ok {
		return err
	}
	pass, err := securerandom.Base64(12)
	if err != nil {
		return err
	}

	admin := user.User{
		Username: "admin",
		Email:    "admins@nitrous.io",
		Password: pass,
	}
	err = admin.Insert(d.(*sqlx.Tx))
	if err != nil {
		return err
	}
	t := token.Token{
		UserID: admin.ID,
	}
	err = t.Insert(d)
	if err != nil {
		return err
	}
	logrus.Infof("Added user %+v", admin)

	return nil
}
