package providertemplates

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	Index               = admin.VerifyAdmin(index)
	Update              = admin.VerifyAdmin(update)
	Show                = admin.VerifyAdmin(show)
	errJSONFormRequired = httperror.New(http.StatusBadRequest,
		"json_form_required", "POST content-type must be application/json")
	errInvalidJSON = httperror.New(http.StatusBadRequest,
		"invalid_json", "Invalid JSON")
)

func index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	pts, err := providertemplate.All(db)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK

	env.Data = map[string]interface{}{
		"provider_templates": serializers.SerializeProviderTemplates(pts),
	}

	return nil
}

func parsePtid(r *http.Request) (int64, error) {
	ptidStr := mux.Vars(r)["ptid"]
	ptid, err := strconv.ParseInt(ptidStr, 10, 64)
	if err != nil {
		return 0, err
	}

	return ptid, nil
}

func getProviderTemplate(env *middleware.Env, r *http.Request) (*providertemplate.ProviderTemplate, error) {
	db := env.DB

	ptid, err := parsePtid(r)
	if err != nil {
		return nil, err
	}

	pt, err := providertemplate.FindByID(db, ptid)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, httperror.New(http.StatusNotFound, "not_found", "not found")
		}
		return nil, err
	}

	return pt, nil
}

func show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	pt, err := getProviderTemplate(env, r)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK

	env.Data = map[string]interface{}{
		"provider_template": serializers.SerializeProviderTemplate(pt),
	}

	return nil
}

type ProviderTemplateForm struct {
	Config map[string]string `json:"config"`
}

func update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return errJSONFormRequired
	}

	db := env.DB

	pt, err := getProviderTemplate(env, r)
	if err != nil {
		return err
	}

	var jsonForm struct {
		Pt struct {
			Config map[string]string `json:"config"`
		} `json:"provider_template"`
	}

	err = json.NewDecoder(r.Body).Decode(&jsonForm)
	if err != nil {
		return errInvalidJSON
	}

	configStore := map[string]sql.NullString{}

	for key, value := range jsonForm.Pt.Config {
		configStore[key] = sql.NullString{value, true}
	}

	pt.Config.Map = configStore

	err = pt.Update(db)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK

	env.Data = map[string]interface{}{
		"provider_template": serializers.SerializeProviderTemplate(pt),
	}

	return nil
}
