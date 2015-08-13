package usersettings

import (
	"net/http"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	settings, err := env.User.Settings(env.DB)
	if err != nil {
		return err
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user_settings": serializers.SerializeUserSettings(settings),
	}
	return nil
}

func Update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	settings, err := env.User.Settings(env.DB)
	if err != nil {
		return err
	}

	if val := r.PostFormValue("auto_close_brackets"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			settings.AutoCloseBrackets = v
		}
	}
	if val := r.PostFormValue("remove_trailing_whitespace_on_save"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			settings.RemoveTrailingWhitespaceOnSave = v
		}
	}
	if val := r.PostFormValue("highlight_line"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			settings.HighlightLine = v
		}
	}
	if val := r.PostFormValue("keybindings"); val != "" {
		settings.Keybindings = val
	}
	if val := r.PostFormValue("match_brackets"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			settings.MatchBrackets = v
		}
	}
	if val := r.PostFormValue("ruler_column"); val != "" {
		if v, err := strconv.ParseInt(val, 10, 64); err == nil {
			settings.RulerColumn = v
		}
	}
	if val := r.PostFormValue("show_line_numbers"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			settings.ShowLineNumbers = v
		}
	}
	if val := r.PostFormValue("show_trailing_space"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			settings.ShowTrailingSpace = v
		}
	}
	if val := r.PostFormValue("soft_tabs"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			settings.SoftTabs = v
		}
	}
	if val := r.PostFormValue("tab_size"); val != "" {
		if v, err := strconv.ParseInt(val, 10, 64); err == nil {
			settings.TabSize = v
		}
	}
	if val := r.PostFormValue("theme"); val != "" {
		settings.Theme = val
	}
	if val := r.PostFormValue("wrap_lines"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			settings.WrapLines = v
		}
	}

	if err := settings.Update(env.DB); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user_settings": serializers.SerializeUserSettings(settings),
	}
	return nil
}
