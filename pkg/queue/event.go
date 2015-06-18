package queue

type Event int

const (
	Create Event = iota
	Update
	Start
	Stop
	Restart
	Terminate
)

func (e Event) IsA(eventName string) bool {
	return e.String() == eventName
}
