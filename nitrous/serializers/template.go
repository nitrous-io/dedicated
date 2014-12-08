package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/template"
)

type Template struct {
	ID          int64      `json:"id"`
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

func SerializeTemplate(t *template.Template) *Template {
	if t == nil {
		return nil
	}
	return &Template{
		ID:          t.ID,
		Slug:        t.Slug,
		Name:        t.Name.String,
		Description: t.Description.String,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func SerializeTemplates(tmpls []*template.Template) []*Template {
	ret := make([]*Template, len(tmpls))
	for i, t := range tmpls {
		ret[i] = SerializeTemplate(t)
	}
	return ret
}
