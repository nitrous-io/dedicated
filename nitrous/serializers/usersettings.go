package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/user"

type UserSettings struct {
	ID     int64 `json:"id"`
	UserID int64 `json:"user_id"`

	AutoCloseBrackets              bool   `json:"auto_close_brackets"`
	HighlightLine                  bool   `json:"highlight_line"`
	Keybindings                    string `json:"keybindings"`
	MatchBrackets                  bool   `json:"match_brackets"`
	RulerColumn                    int64  `json:"ruler_column"`
	ShowLineNumbers                bool   `json:"show_line_numbers"`
	ShowTrailingSpace              bool   `json:"show_trailing_space"`
	SoftTabs                       bool   `json:"soft_tabs"`
	TabSize                        int64  `json:"tab_size"`
	Theme                          string `json:"theme"`
	WrapLines                      bool   `json:"wrap_lines"`
	RemoveTrailingWhitespaceOnSave bool   `json:"remove_trailing_whitespace_on_save"`
}

func SerializeUserSettings(s *user.Settings) *UserSettings {
	return &UserSettings{
		ID:                             s.ID,
		UserID:                         s.UserID,
		AutoCloseBrackets:              s.AutoCloseBrackets,
		HighlightLine:                  s.HighlightLine,
		Keybindings:                    s.Keybindings,
		MatchBrackets:                  s.MatchBrackets,
		RulerColumn:                    s.RulerColumn,
		ShowLineNumbers:                s.ShowLineNumbers,
		ShowTrailingSpace:              s.ShowTrailingSpace,
		SoftTabs:                       s.SoftTabs,
		TabSize:                        s.TabSize,
		Theme:                          s.Theme,
		WrapLines:                      s.WrapLines,
		RemoveTrailingWhitespaceOnSave: s.RemoveTrailingWhitespaceOnSave,
	}
}
