// TODO: merge nitrous/controllers/event with these events
package event

type Event string

const (
	HostCreating     Event = "host.creating"
	HostCreated      Event = "host.created"
	HostCreateFailed Event = "host.create_failed"
	HostStopping     Event = "host.stopping"
	HostStopped      Event = "host.stopped"
	HostUpdated      Event = "host.updated"
	ContainerStopped Event = "container.stopped"
)
