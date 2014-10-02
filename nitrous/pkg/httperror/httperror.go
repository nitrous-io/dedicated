package httperror

import (
	"encoding/json"
	"fmt"
)

type Error struct {
	Code        int    `json:"-"`
	Type        string `json:"error,omitempty"`
	Description string `json:"error_description,omitempty"`
	Hash        string `json:"error_hash,omitempty"`
}

func (e *Error) JSON() ([]byte, error) {
	return json.Marshal(e)
}

func (e *Error) Error() string {
	msg, _ := json.Marshal(e)
	return fmt.Sprintf("HTTPError: %d %s", e.Code, msg)
}

func New(code int, m ...string) error {
	e := &Error{Code: code}
	if len(m) >= 1 {
		e.Type = m[0]
	}
	if len(m) >= 2 {
		e.Description = m[1]
	}
	if len(m) >= 3 {
		e.Hash = m[2]
	}
	return e
}
