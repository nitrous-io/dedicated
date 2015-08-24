package db

import (
	"database/sql"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
)

type Database struct {
	*sqlx.DB
}

type Migration struct {
	Version string
	Up      string
}

const (
	HOST_CREATE_LOCK = iota + 1
	CONTAINER_CREATE_LOCK
	INVOICE_USER_LOCK
	INVOICE_USER_PAY_LOCK
	HOST_SHUTDOWN_LOCK
	IDLE_HOST_LOCK
)

var (
	migrationFileRegex = regexp.MustCompile("^(\\d+)_.+\\.sql$")
)

func (d *Database) WithTransaction(body func(*sqlx.Tx) error) (err error) {
	var tx *sqlx.Tx

	tx, err = d.Beginx()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		} else if e := recover(); e != nil {
			tx.Rollback()
			panic(e)
		} else {
			err = tx.Commit()
		}
	}()

	return body(tx)
}

func NewDatabase(conn string) (*Database, error) {
	db, err := sqlx.Open("postgres", conn)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return &Database{DB: db}, nil
}

func (d *Database) MigrateAll() error {
	migrations := LoadMigrationsFromRootDir()
	return d.Migrate(migrations)
}

func (d *Database) Migrate(migrations []*Migration) error {
	var i int

	err := d.QueryRow(`SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'schema_migrations'`).Scan(&i)
	if err != nil {
		if err == sql.ErrNoRows {
			_, err = d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
				version character varying(255) NOT NULL
			);
			CREATE UNIQUE INDEX unique_schema_migrations ON schema_migrations USING btree (version);`)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	checkVersionExistsStmt, err := d.Prepare(`SELECT 1 FROM schema_migrations WHERE version = $1`)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		version, q := m.Version, m.Up

		err := checkVersionExistsStmt.QueryRow(version).Scan(&i)
		if err == nil {
			continue
		} else if err != nil && err != sql.ErrNoRows {
			return err
		}

		logrus.Infof(`Migrating "%v"...`, version)
		logrus.Info("BEGIN")
		tx, err := d.Begin()
		if err != nil {
			return err
		}

		queries := []func() (sql.Result, error){
			func() (sql.Result, error) {
				return tx.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			},
			func() (sql.Result, error) {
				return tx.Exec(q)
			},
		}
		for _, query := range queries {
			if _, err = query(); err != nil {
				logrus.Info("ROLLBACK")
				errr := tx.Rollback()
				if errr != nil {
					return errr
				}
				return err
			}
		}

		logrus.Info("COMMIT")
		err = tx.Commit()
		if err != nil {
			return err
		}
	}
	return nil
}

func LoadMigrationsFromRootDir() (allMigrations []*Migration) {
	// load existing migrations
	allMigrations = []*Migration{}
	allMigrations = append(allMigrations, legacyMigrations...)

	pth := path.Join(envvars.RootDir, "migrations")
	st, err := os.Stat(pth)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Errorf("Not loading migrations from %q, does not exist", pth)
			return
		}
		logrus.Errorf("Not loading migrations from %q, error: %v", err)
		return
	}
	if !st.IsDir() {
		logrus.Errorf("Not loading migrations from %q, not a directory", err)
		return
	}

	files, err := ioutil.ReadDir(pth)
	if err != nil {
		logrus.Errorf("Not loading migrations from %q, error: %v", err)
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		m, err := LoadMigrationFile(pth, file.Name())
		if file.Name() == "README.md" {
			continue
		}
		if err != nil {
			logrus.Errorf("Error loading %q, %v", file.Name(), err)
			continue
		}
		allMigrations = append(allMigrations, m)
	}
	return
}

func LoadMigrationFile(pth, file string) (*Migration, error) {
	matches := migrationFileRegex.FindStringSubmatch(file)
	if len(matches) == 0 {
		return nil, errors.New("does not match pattern")
	}
	logrus.Infof("Loading migration %s", file)
	fullpath := path.Join(pth, file)
	contents, err := ioutil.ReadFile(fullpath)
	if err != nil {
		return nil, err
	}

	contentsStr := strings.TrimSpace(string(contents[:]))
	if len(contentsStr) == 0 {
		return nil, errors.New("Migration is empty")
	}

	m := &Migration{
		Version: matches[1],
		Up:      contentsStr,
	}
	return m, nil
}

// TransactionLock tries to set an advisory lock on transaction and
// returns its result
func TransactionLock(tx *sqlx.Tx, op, id int) (bool, error) {
	var lockObtained bool
	err := tx.QueryRow("SELECT pg_try_advisory_xact_lock($1, $2);", op, id).Scan(&lockObtained)
	return lockObtained, err
}
