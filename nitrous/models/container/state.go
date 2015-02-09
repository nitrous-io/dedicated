package container

type State int

const (
	StateUnknown State = iota // It's important that "unknown" is 0.
	StateRunning
	StateStopped
	StateTerminated
	StateHostStopped

	StateStarting
	StateStopping
	StateRestarting
	StateReconfiguring
	StateTerminating
)

func (s State) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StateStopped:
		return "stopped"
	case StateTerminated:
		return "terminated"
	case StateHostStopped:
		return "host_stopped"
	case StateStarting:
		return "starting"
	case StateStopping:
		return "stopping"
	case StateRestarting:
		return "restarting"
	case StateReconfiguring:
		return "reconfiguring"
	case StateTerminating:
		return "terminating"
	default:
		return "unknown"
	}
}
