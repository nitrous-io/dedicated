// TODO: merge nitrous/controllers/event with these events
package event

type Event string

const (
	HostCreating     Event = "host.creating"
	HostCreated      Event = "host.created"
	HostCreateFailed Event = "host.create_failed"
)
