package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/user"

type UserSettings struct {
	ID     int64 `json:"id"`
	UserID int64 `json:"user_id"`

	AutoCloseBrackets bool   `json:"auto_close_brackets"`
	HighlightLine     bool   `json:"highlight_line"`
	Keybindings       string `json:"keybindings"`
	MatchBrackets     bool   `json:"match_brackets"`
	RulerColumn       int64  `json:"ruler_column"`
	ShowLineNumbers   bool   `json:"show_line_numbers"`
	ShowTrailingSpace bool   `json:"show_trailing_space"`
	SoftTabs          bool   `json:"soft_tabs"`
	TabSize           int64  `json:"tab_size"`
	Theme             string `json:"theme"`
	WrapLines         bool   `json:"wrap_lines"`
}

func SerializeUserSettings(s *user.Settings) *UserSettings {
	serialized := &UserSettings{
		ID:                s.ID,
		UserID:            s.UserID,
		AutoCloseBrackets: s.AutoCloseBrackets,
		HighlightLine:     s.HighlightLine,
		MatchBrackets:     s.MatchBrackets,
		RulerColumn:       s.RulerColumn,
		ShowLineNumbers:   s.ShowLineNumbers,
		ShowTrailingSpace: s.ShowTrailingSpace,
		SoftTabs:          s.SoftTabs,
		TabSize:           s.TabSize,
		WrapLines:         s.WrapLines,
	}
	if s.Keybindings.Valid {
		serialized.Keybindings = s.Keybindings.String
	}
	if s.Theme.Valid {
		serialized.Theme = s.Theme.String
	}
	return serialized
}
