package admin

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errSlugRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "slug required")
	errNameRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "name required")
	errImageRequired    = httperror.New(http.StatusBadRequest, "invalid_request", "image required")
	errInvalidType      = httperror.New(http.StatusBadRequest, "invalid_request", "invalid type")
	errConfigInvalid    = httperror.New(http.StatusBadRequest, "invalid_request", "config invalid")
	errTemplateNotFound = httperror.New(http.StatusNotFound, "not_found", "template not found")

	TemplatesCreate = VerifyAdmin(templatesCreate)
	TemplateShow    = VerifyAdmin(templateShow)
	TemplateUpdate  = VerifyAdmin(templateUpdate)
)

func templatesCreate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	slug := r.PostFormValue("slug")
	if slug == "" {
		return errSlugRequired
	}

	name := r.PostFormValue("name")
	if name == "" {
		return errNameRequired
	}

	image := r.PostFormValue("image")
	if image == "" {
		return errImageRequired
	}

	tmpl := &template.Template{
		Slug:        slug,
		Name:        sql.NullString{name, true},
		Description: sql.NullString{r.PostFormValue("description"), true},
		Config:      r.PostFormValue("config"),
		Type:        r.PostFormValue("type"),
		Image:       image,
	}
	err := tmpl.Insert(db)
	if err != nil {
		if err == template.ErrInvalidType {
			return errInvalidType
		}
		if err == template.ErrConfigInvalid {
			return errConfigInvalid
		}
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"template": serializers.SerializeTemplate(tmpl),
	}
	return nil
}

func findTemplate(d *db.Database, slug string) (*template.Template, error) {
	tmpl, err := template.FindBySlug(d, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errTemplateNotFound
		}
		if err == template.ErrConfigInvalid {
			return nil, errConfigInvalid
		}
		return nil, err
	}
	return tmpl, nil
}

func templateShow(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	tmpl, err := findTemplate(env.DB, mux.Vars(r)["slug"])
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"template": serializers.SerializeAdminTemplate(tmpl),
	}
	return nil
}

func templateUpdate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	tmpl, err := findTemplate(env.DB, mux.Vars(r)["slug"])
	if err != nil {
		return err
	}

	slug := r.PostFormValue("slug")
	if slug == "" {
		return errSlugRequired
	}

	name := r.PostFormValue("name")
	if name == "" {
		return errNameRequired
	}

	image := r.PostFormValue("image")
	if image == "" {
		return errImageRequired
	}

	tmpl.Slug = slug
	tmpl.Name = sql.NullString{name, true}
	tmpl.Description = sql.NullString{r.PostFormValue("description"), true}
	tmpl.Config = r.PostFormValue("config")
	tmpl.Type = r.PostFormValue("type")
	tmpl.Image = image

	err = tmpl.Update(env.DB)
	if err != nil {
		if err == template.ErrInvalidType {
			return errInvalidType
		}
		if err == template.ErrConfigInvalid {
			return errConfigInvalid
		}
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"template": serializers.SerializeTemplate(tmpl),
	}

	return nil
}
