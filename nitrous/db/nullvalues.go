package db

import (
	"database/sql"
	"encoding/json"
)

// NullString embeds an sql.NullString and implements json.Marshaler so that
// we can render JSON like:
//   `key: "nano"`
// instead of
//   `key: {"String":"nano","Valid":true}`
type NullString struct {
	sql.NullString
}

func (s *NullString) MarshalJSON() ([]byte, error) {
	if !s.Valid {
		return json.Marshal(nil)
	}
	return json.Marshal(s.String)
}
