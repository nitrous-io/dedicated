package usersettings

import (
	"encoding/json"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
)

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	settings, err := env.User.Settings(env.DB)
	if err != nil {
		return err
	}

	m, err := settings.IdeSettingsMapWithDefaults()
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user_settings": m,
	}
	return nil
}

func Update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	settings, err := env.User.Settings(env.DB)
	if err != nil {
		return err
	}

	existing, err := settings.IdeSettingsMap()
	if err != nil {
		return err
	}

	newData := map[string]interface{}{}
	if err := json.NewDecoder(r.Body).Decode(&newData); err != nil {
		return err
	}

	for k, v := range newData {
		existingVal, ok := existing[k]
		// we don't want to store existing values
		if ok && existingVal == v {
			continue
		}
		existing[k] = v
	}

	jsonBytes, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	settings.IdeSettings = string(jsonBytes[:])

	if err := settings.Update(env.DB); err != nil {
		return err
	}

	m, err := settings.IdeSettingsMapWithDefaults()
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user_settings": m,
	}
	return nil
}
