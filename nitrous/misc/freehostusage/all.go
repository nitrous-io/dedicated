package freehostusage

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/nitrous-common/logger"
)

func ScanAllFreeHosts(d *db.Database, logr *logger.Logger, action HostAction, now time.Time, dryrun bool) {
	ScanWarningHosts(d, logr, action, now, dryrun)
	TerminateInactiveFreeHosts(d, logr, action, now, dryrun)
}
