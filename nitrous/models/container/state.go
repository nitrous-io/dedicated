package container

const (
	StateUnknown       = "unknown"
	StateCreating      = "creating"
	StateCreateFailed  = "create_failed"
	StateStarting      = "starting"
	StateRunning       = "running"
	StateRestarting    = "restarting"
	StateStopping      = "stopping"
	StateStopped       = "stopped"
	StateTerminating   = "terminating"
	StateTerminated    = "terminated"
	StateReconfiguring = "reconfiguring"
)
