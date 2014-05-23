package datastore

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nitrous-io/freighter/util"
)

type Datastore struct {
	Conn   *sql.DB
	Logger *util.Logger
}

type KeyVal struct {
	Key string
	Val string
}

func NewSqliteDatastore(dbfile string) (*Datastore, error) {
	conn, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		return nil, err
	}
	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS store (
			key TEXT PRIMARY KEY NOT NULL,
			val TEXT
		)`)
	if err != nil {
		return nil, err
	}
	return &Datastore{Conn: conn}, nil
}

func (d *Datastore) Set(key string, val string) error {
	_, err := d.Conn.Exec("INSERT OR REPLACE INTO store (key, val) VALUES (?, ?)", key, val)
	return err
}

func (d *Datastore) Update(key string, val string) error {
	_, err := d.Conn.Exec("UPDATE store SET val = ? WHERE key = ?", val, key)
	return err
}

func (d *Datastore) Get(key string) (string, error) {
	var val string
	err := d.Conn.QueryRow("SELECT val FROM store WHERE key = ?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (d *Datastore) Find(pattern string) ([]*KeyVal, error) {
	return d.query(func() (*sql.Rows, error) {
		return d.Conn.Query("SELECT key, val FROM store WHERE key LIKE ? ORDER BY key ASC", pattern)
	})
}

func (d *Datastore) FindOne(pattern string) (*KeyVal, error) {
	kvs, err := d.query(func() (*sql.Rows, error) {
		return d.Conn.Query("SELECT key, val FROM store WHERE key LIKE ? ORDER BY key ASC LIMIT 1", pattern)
	})
	if err != nil {
		return nil, err
	}
	if len(kvs) == 0 {
		return nil, nil
	}
	return kvs[0], nil
}

func (d *Datastore) FindWhere(pattern string, val string) ([]*KeyVal, error) {
	return d.query(func() (*sql.Rows, error) {
		return d.Conn.Query("SELECT key, val FROM store WHERE key LIKE ? AND val = ? ORDER BY key ASC", pattern, val)
	})
}

func (d *Datastore) FindOneWhere(pattern string, val string) (*KeyVal, error) {
	kvs, err := d.query(func() (*sql.Rows, error) {
		return d.Conn.Query("SELECT key, val FROM store WHERE key LIKE ? AND val = ? ORDER BY key ASC LIMIT 1", pattern, val)
	})
	if err != nil {
		return nil, err
	}
	if len(kvs) == 0 {
		return nil, nil
	}
	return kvs[0], nil
}

func (d *Datastore) query(queryFn func() (*sql.Rows, error)) ([]*KeyVal, error) {
	kvs := []*KeyVal{}
	rows, err := queryFn()
	if err == sql.ErrNoRows {
		return kvs, nil
	} else if err != nil {
		return nil, err
	}

	for rows.Next() {
		var key string
		var val string
		err = rows.Scan(&key, &val)
		if err != nil {
			return nil, err
		}
		kvs = append(kvs, &KeyVal{Key: key, Val: val})
	}

	return kvs, nil
}
