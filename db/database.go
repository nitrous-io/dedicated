package db

import (
	"database/sql"

	_ "github.com/lib/pq"
	"github.com/nitrous-io/freighter/util"
)

type Database struct {
	*sql.DB
	Logger *util.Logger
}

type Migration struct {
	Version string
	Up      string
}

func NewDatabase(conn string) (*Database, error) {
	db, err := sql.Open("postgres", conn)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return &Database{DB: db}, nil
}

func (d *Database) Migrate(migrations []*Migration) error {
	logger := d.Logger

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

		if logger != nil {
			logger.Infof(`Migrating "%v"...`, version)
			logger.Info("BEGIN")
		}
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
