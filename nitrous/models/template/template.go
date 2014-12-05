package template

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/nitrous-io/nitrous/db"
)

var ErrConfigInvalid = errors.New("template: .Config is invalid")

type Template struct {
	ID          int64          `db:"id"`
	Slug        string         `db:"slug"`
	Name        sql.NullString `db:"name"`
	Description sql.NullString `db:"description"`
	Config      string         `db:"config"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// TemplateConfig is used for unmarshalling Template.Config.
type TemplateConfig struct {
	Containers []Container `json:"containers"`
	Volumes    []Volume    `json:"volumes"`
}

type Container struct {
	Name       string   `json:"name"`
	Slug       string   `json:"slug"`
	Image      string   `json:"image"`
	Cmd        string   `json:"cmd"`
	Privileged bool     `json:"privileged"`
	PublishAll bool     `json:"publish_all"`
	Env        []string `json:"env,omitempty"`
	Ports      []string `json:"ports,omitempty"`
	Volumes    []string `json:"volumes,omitempty"`
}

type Volume struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func All(d db.QueryReceiver) ([]*Template, error) {
	tmpls := []*Template{}
	if err := d.Select(&tmpls, `SELECT * FROM templates ORDER BY id;`); err != nil {
		return nil, err
	}
	return tmpls, nil
}

func Find(d db.QueryReceiver, slug string) (*Template, error) {
	tmpl := &Template{}
	err := d.Get(tmpl, `SELECT * FROM templates WHERE slug = $1;`, slug)
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

func (t *Template) ParsedConfig() (*TemplateConfig, error) {
	var cfg TemplateConfig
	if err := json.Unmarshal([]byte(t.Config), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (t *Template) Insert(d db.QueryReceiver) error {
	// Make sure we can unmarshal the template's Config.
	if _, err := t.ParsedConfig(); err != nil {
		return ErrConfigInvalid
	}

	nstmt, err := d.PrepareNamed(`INSERT INTO templates
		(slug, name, description, config)
		VALUES (:slug, :name, :description, :config) RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(t).StructScan(t)
}
