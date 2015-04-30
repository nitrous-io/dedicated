package event

var events = []string{
	"CreatingImage",
	"CreatedImage",
	"FailedToCreateImage",
	"CreatedHost",
	"FailedToCreateHost",
}

type Event int

const (
	CreatingImage Event = iota
	CreatedImage
	FailedToCreateImage
	CreatedHost
	FailedToCreateHost
)

func (e Event) String() string {
	return events[e]
}
