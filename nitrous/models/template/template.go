package template

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

const (
	TypeBasic    = "basic"
	TypeStack    = "stack"
	TypeAdvanced = "advanced"
)

var (
	AllowedTypes = []string{TypeBasic, TypeStack, TypeAdvanced}
)

var (
	ErrInvalidType   = errors.New("`type` not known")
	ErrConfigInvalid = errors.New("template: .Config is invalid")
)

type Template struct {
	ID          int64          `db:"id"`
	Slug        string         `db:"slug"`
	Name        sql.NullString `db:"name"`
	Description sql.NullString `db:"description"`
	Type        string         `db:"type"`
	Config      string         `db:"config"`
	Image       string         `db:"image"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// TemplateConfig is used for unmarshalling Template.Config.
type TemplateConfig struct {
	Privileged bool     `json:"privileged"`
	PublishAll bool     `json:"publish_all"`
	Env        []string `json:"env,omitempty"`
	Ports      []string `json:"ports,omitempty"`
	Volumes    []string `json:"volumes,omitempty"`
}

func All(d db.QueryReceiver) ([]*Template, error) {
	tmpls := []*Template{}
	if err := d.Select(&tmpls, `SELECT * FROM templates ORDER BY id;`); err != nil {
		return nil, err
	}
	return tmpls, nil
}

func FindBySlug(d db.QueryReceiver, slug string) (*Template, error) {
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

func (t *Template) validType() bool {
	ok := false
	for _, ty := range AllowedTypes {
		if t.Type == ty {
			ok = true
			break
		}
	}

	return ok
}

func (t *Template) Insert(d db.QueryReceiver) error {
	// Make sure we can unmarshal the template's Config.
	if _, err := t.ParsedConfig(); err != nil {
		return ErrConfigInvalid
	}

	if !t.validType() {
		return ErrInvalidType
	}

	nstmt, err := d.PrepareNamed(`INSERT INTO templates
		(slug, name, description, config, type, image)
		VALUES (:slug, :name, :description, :config, :type, :image) RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(t).StructScan(t)
}

func (t *Template) Update(d db.QueryReceiver) error {
	// Make sure we can unmarshal the template's Config.
	if _, err := t.ParsedConfig(); err != nil {
		return ErrConfigInvalid
	}

	if !t.validType() {
		return ErrInvalidType
	}

	nstmt, err := d.PrepareNamed(`UPDATE templates
		SET (slug, name, description, config, type, image) = (:slug, :name, :description, :config, :type, :image)
		WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(t).StructScan(t)
}
