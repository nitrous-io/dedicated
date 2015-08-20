package user

import (
	"encoding/json"

	"github.com/lib/pq"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
)

type Settings struct {
	ID     int64 `db:"id"`
	UserID int64 `db:"user_id"`

	IdeSettings string `db:"ide_settings"`

	Migrated bool `db:"migrated"`

	// DEPRECATED (kept around for migrating)
	AutoCloseBrackets              bool   `db:"auto_close_brackets"`
	HighlightLine                  bool   `db:"highlight_line"`
	Keybindings                    string `db:"keybindings"`
	MatchBrackets                  bool   `db:"match_brackets"`
	RulerColumn                    int64  `db:"ruler_column"`
	ShowLineNumbers                bool   `db:"show_line_numbers"`
	ShowTrailingSpace              bool   `db:"show_trailing_space"`
	SoftTabs                       bool   `db:"soft_tabs"`
	TabSize                        int64  `db:"tab_size"`
	Theme                          string `db:"theme"`
	WrapLines                      bool   `db:"wrap_lines"`
	RemoveTrailingWhitespaceOnSave bool   `db:"remove_trailing_whitespace_on_save"`

	CreatedAt pq.NullTime `db:"created_at"`
	UpdatedAt pq.NullTime `db:"updated_at"`
}

var (
	DefaultSettings = map[string]interface{}{
		"auto_close_brackets":                true,
		"highlight_line":                     true,
		"keybindings":                        "default",
		"match_brackets":                     true,
		"ruler_column":                       0,
		"show_line_numbers":                  true,
		"show_trailing_space":                false,
		"soft_tabs":                          true,
		"tab_size":                           2,
		"theme":                              "solarized dark",
		"wrap_lines":                         true,
		"remove_trailing_whitespace_on_save": true,
	}
)

func (s *Settings) MigrateDivergedSettings(db ndb.QueryReceiver) error {
	if s.Migrated {
		return nil
	}

	userSettings := map[string]interface{}{
		"auto_close_brackets":                s.AutoCloseBrackets,
		"highlight_line":                     s.HighlightLine,
		"keybindings":                        s.Keybindings,
		"match_brackets":                     s.MatchBrackets,
		"ruler_column":                       s.RulerColumn,
		"show_line_numbers":                  s.ShowLineNumbers,
		"show_trailing_space":                s.ShowTrailingSpace,
		"soft_tabs":                          s.SoftTabs,
		"tab_size":                           s.TabSize,
		"theme":                              s.Theme,
		"wrap_lines":                         s.WrapLines,
		"remove_trailing_whitespace_on_save": s.RemoveTrailingWhitespaceOnSave,
	}

	diff := map[string]interface{}{}
	for k, v := range DefaultSettings {
		userVal := userSettings[k]
		if v != userVal {
			diff[k] = userVal
		}
	}

	if len(diff) == 0 {
		s.Migrated = true
		return s.Update(db)
	}

	// perform migration
	jsonBytes, err := json.Marshal(diff)
	if err != nil {
		return err
	}
	s.Migrated = true
	s.IdeSettings = string(jsonBytes[:])
	return s.Update(db)
}

func (s *Settings) IdeSettingsMap() (map[string]interface{}, error) {
	m := map[string]interface{}{}
	if err := json.Unmarshal([]byte(s.IdeSettings), &m); err != nil {
		return nil, err
	}

	return m, nil
}

func (s *Settings) IdeSettingsMapWithDefaults() (map[string]interface{}, error) {
	m, err := s.IdeSettingsMap()
	if err != nil {
		return nil, err
	}

	for k, v := range DefaultSettings {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}

	return m, nil
}

func (s *Settings) Update(db ndb.QueryReceiver) error {
	nstmt, err := db.PrepareNamed(`UPDATE user_settings
		SET (ide_settings, migrated, auto_close_brackets, highlight_line, keybindings, match_brackets,
			ruler_column, show_line_numbers, show_trailing_space, soft_tabs,
			tab_size, theme, wrap_lines, remove_trailing_whitespace_on_save, updated_at)
                = (:ide_settings, :migrated, :auto_close_brackets, :highlight_line, :keybindings, :match_brackets,
			:ruler_column, :show_line_numbers, :show_trailing_space, :soft_tabs,
			:tab_size, :theme, :wrap_lines, :remove_trailing_whitespace_on_save, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(s).StructScan(s)
}

func (u *User) CreateSettings(db ndb.QueryReceiver) error {
	_, err := db.Exec(`INSERT INTO user_settings (user_id) VALUES ($1);`, u.ID)
	return err
}

func (u *User) Settings(db ndb.QueryReceiver) (*Settings, error) {
	s := &Settings{}
	err := db.Get(s, `SELECT * FROM user_settings WHERE user_id = $1`, u.ID)
	if err != nil {
		return nil, err
	}
	return s, nil
}
