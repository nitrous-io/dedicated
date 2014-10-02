package container

type State int

const (
	StateUnknown State = iota // It's important that "unknown" is 0.
	StateRunning
	StateStopped
	StateTerminated
	StateHostStopped
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
	default:
		return "unknown"
	}
}
