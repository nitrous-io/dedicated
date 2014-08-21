package container

type State int

const (
	StateUnknown State = iota // It's important that "unknown" is 0.
	StateRunning
	StateStopped
	StateTerminated
)

func (s State) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StateStopped:
		return "stopped"
	case StateTerminated:
		return "terminated"
	default:
		return "unknown"
	}
}
