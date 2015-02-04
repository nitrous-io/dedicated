package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/template"
)

type Template struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Privileged  bool   `json:"privileged"`
	Image       string `json:"image"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeTemplate(t *template.Template) *Template {
	if t == nil {
		return nil
	}
	base := &Template{
		ID:          t.ID,
		Slug:        t.Slug,
		Name:        t.Name.String,
		Description: t.Description.String,
		Type:        t.Type,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		Image:       t.Image,
		Privileged:  false,
	}

	conf, err := t.ParsedConfig()
	if err == nil {
		base.Privileged = conf.Privileged
	}
	return base
}

func SerializeTemplates(tmpls []*template.Template) []*Template {
	ret := make([]*Template, len(tmpls))
	for i, t := range tmpls {
		ret[i] = SerializeTemplate(t)
	}
	return ret
}
