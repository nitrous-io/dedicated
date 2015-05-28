package admin

import (
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/nitrous-common/logger"
)

type tFunc func(tx *sqlx.Tx, l *logger.Logger, h *host.Host, headers http.Header, initiatorID int64) error

func SetTerminateFunc(t tFunc) {
	terminateFunc = t
}
