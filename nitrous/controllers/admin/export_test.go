package admin

import (
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type tFunc func(tx *sqlx.Tx, h *host.Host, headers http.Header, initiatorID int64, origState string) error

func SetTerminateFunc(t tFunc) {
	terminateFunc = t
}
