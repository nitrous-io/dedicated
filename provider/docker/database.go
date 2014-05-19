package docker

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nitrous-io/freighter/util"
)

type Database struct {
	Db     *sql.DB
	Logger *util.Logger
}

func NewSqliteDatabase(dbfile string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		return nil, err
	}
	return &Database{Db: db}, nil
}

func (d *Database) Migrate(migrations [][]string) error {
	db := d.Db
	logger := d.Logger

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) NOT NULL
	);
	CREATE UNIQUE INDEX unique_schema_migrations ON schema_migrations (version)`)
	if err != nil {
		return err
	}

	checkVersionExistsStmt, err := db.Prepare(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)`)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		version, q := m[0], m[1]

		var exists bool
		err := checkVersionExistsStmt.QueryRow(version).Scan(&exists)
		if err != nil {
			return err
		}

		if exists {
			continue
		}

		if logger != nil {
			logger.Infof(`Migrating "%v"...`, version)
			logger.Info("BEGIN")
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}

		queries := []func() (sql.Result, error){
			func() (sql.Result, error) {
				return tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version)
			},
			func() (sql.Result, error) {
				return tx.Exec(q)
			},
		}
		for _, query := range queries {
			if _, err = query(); err != nil {
				if logger != nil {
					logger.Info("ROLLBACK")
				}
				errr := tx.Rollback()
				if errr != nil {
					return errr
				}
				return err
			}
		}

		if logger != nil {
			logger.Info("COMMIT")
		}
		err = tx.Commit()
		if err != nil {
			return err
		}
	}
	return nil
}
