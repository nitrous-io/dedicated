package usersettings

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	settings, err := env.User.Settings(env.DB)
	if err != nil {
		return err
	}

	if err := settings.MigrateDivergedSettings(env.DB); err != nil {
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
	// TODO: Remove me (after August 22 2015) when we are confident everyone is using an updated IDE
	// as we will begin enforcing JSON
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&newData); err != nil {
			return err
		}
	}

	// TODO: Remove me (after August 22 2015) when we are confident everyone is using an updated IDE
	handleLegacyParams(r, newData)

	for k, v := range newData {
		existingVal, ok := user.DefaultSettings[k]
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

func handleLegacyParams(r *http.Request, m map[string]interface{}) {
	if val := r.PostFormValue("auto_close_brackets"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			m["auto_close_brackets"] = v
		}
	}
	if val := r.PostFormValue("remove_trailing_whitespace_on_save"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			m["remove_trailing_whitespace_on_save"] = v
		}
	}
	if val := r.PostFormValue("highlight_line"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			m["highlight_line"] = v
		}
	}
	if val := r.PostFormValue("keybindings"); val != "" {
		m["keybindings"] = val
	}
	if val := r.PostFormValue("match_brackets"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			m["match_brackets"] = v
		}
	}
	if val := r.PostFormValue("ruler_column"); val != "" {
		if v, err := strconv.ParseInt(val, 10, 64); err == nil {
			m["ruler_column"] = v
		}
	}
	if val := r.PostFormValue("show_line_numbers"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			m["show_line_numbers"] = v
		}
	}
	if val := r.PostFormValue("show_trailing_space"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			m["show_trailing_space"] = v
		}
	}
	if val := r.PostFormValue("soft_tabs"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			m["soft_tabs"] = v
		}
	}
	if val := r.PostFormValue("tab_size"); val != "" {
		if v, err := strconv.ParseInt(val, 10, 64); err == nil {
			m["tab_size"] = v
		}
	}
	if val := r.PostFormValue("theme"); val != "" {
		m["theme"] = val
	}
	if val := r.PostFormValue("wrap_lines"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			m["wrap_lines"] = v
		}
	}
}
