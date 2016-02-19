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
		if curVal, ok := m[k]; !ok || curVal == nil {
			m[k] = v
		}
	}

	return m, nil
}

func (s *Settings) Update(db ndb.QueryReceiver) error {
	nstmt, err := db.PrepareNamed(`UPDATE user_settings
		SET (ide_settings, updated_at) = (:ide_settings, now())
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
