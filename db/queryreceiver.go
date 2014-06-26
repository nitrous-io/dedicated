package db

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// sqlx.Tx and sqlx.DB satisfy this interface

type QueryReceiver interface {
	sqlx.Execer
	sqlx.Queryer

	NamedQuery(query string, arg interface{}) (*sqlx.Rows, error)
	NamedExec(query string, arg interface{}) (sql.Result, error)
	Select(dest interface{}, query string, args ...interface{}) error
	Get(dest interface{}, query string, args ...interface{}) error
}
