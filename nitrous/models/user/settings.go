package user

import (
	"github.com/lib/pq"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
)

type Settings struct {
	ID     int64 `db:"id"`
	UserID int64 `db:"user_id"`

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

func (s *Settings) Update(db ndb.QueryReceiver) error {
	nstmt, err := db.PrepareNamed(`UPDATE user_settings
		SET (auto_close_brackets, highlight_line, keybindings, match_brackets,
			ruler_column, show_line_numbers, show_trailing_space, soft_tabs,
			tab_size, theme, wrap_lines, remove_trailing_whitespace_on_save, updated_at)
		= (:auto_close_brackets, :highlight_line, :keybindings, :match_brackets,
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
