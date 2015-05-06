package event

var events = []string{
	"host.creating",
	"image.created",
	"image.build_failed",
	"host.created",
	"host.create_failed",
}

type Event int

const (
	CreatingImage Event = iota
	CreatedImage
	FailedToCreateImage
	CreatedHost
	FailedToCreateHost
)
